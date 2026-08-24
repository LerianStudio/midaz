// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// transformedCRMCodeRegex matches the CRM-00xx codes that the deleted
// ErrorCodeTransformer shim used to rewrite canonical midaz codes into. A
// response carrying any of these on a formerly-transformed path proves the shim
// (or a regression of it) is back, so every contract assertion below checks the
// code does NOT match this pattern.
var transformedCRMCodeRegex = regexp.MustCompile(`^CRM-00(01|02|03|04|05|07|09|11|12|14|15|16|18)$`)

// TestErrorContract_CanonicalCodes locks the post-shim wire contract: CRM error
// responses on the formerly-transformed paths now carry canonical midaz codes,
// NOT the CRM-00xx codes the deleted ErrorCodeTransformer used to rewrite them
// into (PD-2). Each canonical code below was confirmed against the live handler
// at authoring time and is pinned exactly — there is no "set of acceptable
// codes" fallback. If the shim is ever reintroduced, these assertions break.
//
// Path -> formerly-transformed CRM code -> canonical midaz code now emitted:
//   - missing required field   CRM-0003 -> 0009 (ErrMissingFieldsInRequest)
//   - malformed request body    CRM-0004 -> 0094 (ErrInvalidRequestBody, the
//     non-1:1 mapping the shim performed)
//   - invalid holder type       CRM-0015 -> 0047 (ErrBadRequest)
//   - internal/repository error CRM-0014 -> 0046 (passed through unchanged)
//   - unexpected fields         CRM-0007 -> 0053 (ErrUnexpectedFieldsInTheRequest)
func TestErrorContract_CanonicalCodes(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(holderRepo *holder.MockRepository, orgID string)
		expectedStatus int
		expectedCode   string
		expectedField  string
	}{
		{
			name:     "missing required fields emits canonical 0009 not CRM-0003",
			jsonBody: `{"name":"John Doe","document":"91315026015"}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// Validation fails before the repository is reached.
			},
			expectedStatus: 400,
			expectedCode:   "0009",
		},
		{
			name:     "malformed request body emits canonical 0094 not CRM-0004",
			jsonBody: `{"type": }`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// Unmarshalling fails before the repository is reached.
			},
			expectedStatus: 400,
			expectedCode:   "0094",
		},
		{
			name:     "invalid holder type emits canonical 0047 not CRM-0015",
			jsonBody: `{"type":"INVALID","name":"John Doe","document":"91315026015"}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// Validation fails before the repository is reached.
			},
			expectedStatus: 400,
			expectedCode:   "0047",
			expectedField:  "type",
		},
		{
			name:     "lowercase holder type emits canonical 0047 not CRM-0015",
			jsonBody: `{"type":"natural_person","name":"John Doe","document":"91315026015"}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// Validation fails before the repository is reached.
			},
			expectedStatus: 400,
			expectedCode:   "0047",
			expectedField:  "type",
		},
		{
			name:     "internal server error emits canonical 0046 not CRM-0014",
			jsonBody: `{"type":"NATURAL_PERSON","name":"John Doe","document":"91315026015"}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				holderRepo.EXPECT().
					Create(gomock.Any(), orgID, gomock.Any()).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			expectedCode:   "0046",
		},
		{
			name:     "unexpected fields emit canonical 0053 not CRM-0007",
			jsonBody: `{"type":"NATURAL_PERSON","name":"John Doe","document":"91315026015","bogusField":"x"}`,
			setupMocks: func(holderRepo *holder.MockRepository, orgID string) {
				// Unknown-field detection fails before the repository is reached.
			},
			expectedStatus: 400,
			expectedCode:   "0053",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.Must(libCommons.GenerateUUIDv7()).String()

			handler, mockHolderRepo := newHolderHandler(t, ctrl)
			tt.setupMocks(mockHolderRepo, orgID)

			app := buildHumaHolderApp(t, handler, true)

			req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID+"/holders", bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			require.NoError(t, json.Unmarshal(body, &errResp))

			code, ok := errResp["code"].(string)
			require.True(t, ok, "error response must carry a string code field, got: %s", string(body))

			assert.Equal(t, tt.expectedCode, code,
				"path must emit the exact canonical midaz code (no CRM-00xx shim rewrite)")
			assert.NotRegexp(t, transformedCRMCodeRegex, code,
				"response code must NOT be a formerly-transformed CRM-00xx code")

			if tt.expectedField != "" {
				// RFC 9457 carries per-field detail in the "errors" array, each entry
				// naming the offending field in "location".
				errs, ok := errResp["errors"].([]any)
				require.True(t, ok, "validation error response must carry an errors array, got: %s", string(body))

				var message string

				for _, e := range errs {
					entry, ok := e.(map[string]any)
					if !ok {
						continue
					}

					if entry["location"] == tt.expectedField {
						message, _ = entry["message"].(string)
						break
					}
				}

				assert.NotEmpty(t, message, "validation error response must explain the invalid field %q", tt.expectedField)
			}
		})
	}
}

// TestErrorContract_SurvivingDomainCodeUnchanged asserts the deletion of the
// transform shim did not disturb the live CRM domain sentinels: a genuine CRM
// domain error (holder-not-found, CRM-0006) still surfaces with its CRM-00xx
// code unchanged. The pruned transform codes and the surviving domain codes are
// two distinct sets; this guards the latter.
func TestErrorContract_SurvivingDomainCodeUnchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, mockHolderRepo := newHolderHandler(t, ctrl)
	mockHolderRepo.EXPECT().
		Find(gomock.Any(), orgID, holderID, false).
		Return(nil, pkg.ValidateBusinessError(cn.ErrHolderNotFound, cn.EntityHolder)).
		Times(1)

	app := buildHumaHolderApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID+"/holders/"+holderID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(body, &errResp))

	assert.Equal(t, cn.ErrHolderNotFound.Error(), errResp["code"],
		"surviving CRM domain sentinel CRM-0006 must be emitted unchanged")
	assert.Equal(t, "CRM-0006", errResp["code"],
		"CRM-0006 wire code is part of the external contract and must not drift")
}
