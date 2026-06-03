// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	netHttp "net/http"
	"strconv"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// CountParams holds the parameters for counting transactions by route on the Midaz transaction service.
type CountParams struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Route          string
	Status         string
	StartDate      time.Time
	EndDate        time.Time
}

// AccountFilters holds optional filter parameters for listing accounts on the Midaz onboarding service.
type AccountFilters struct {
	SegmentID   *uuid.UUID
	PortfolioID *uuid.UUID
}

// AccountPage represents a paginated response of accounts from the Midaz onboarding service.
type AccountPage struct {
	Items []pkg.Account `json:"items"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
}

//go:generate mockgen --destination=./midaz_service_mock.go --package=http . MidazClient
type MidazClient interface {
	GetAccountFromMidazByAlias(ctx context.Context, creditAccount, organizationID, ledgerID string) error
	GetAccountDetailsByAlias(ctx context.Context, organizationID, ledgerID, alias string) (*pkg.Account, error)
	CountTransactionsByRoute(ctx context.Context, params CountParams) (int64, error)
	ListAccounts(ctx context.Context, orgID, ledgerID uuid.UUID, filters AccountFilters, page, limit int) (*AccountPage, error)
}

// AuthTokenGetter interface for getting application tokens
type AuthTokenGetter interface {
	GetApplicationToken(ctx context.Context, clientID, clientSecret string) (string, error)
}

// authClientAdapter adapter for AuthClient
type authClientAdapter struct {
	authClient *middleware.AuthClient
}

func (a *authClientAdapter) GetApplicationToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	return a.authClient.GetApplicationToken(ctx, clientID, clientSecret)
}

// midazErrorBody is used to detect whether a 404 response is a structured Midaz
// business error (account not found) or a routing error (wrong URL, HTML page, etc.).
type midazErrorBody struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// isMidazAccountNotFound reads the response body of a 404 and returns true only when
// the body is a valid Midaz error JSON with a non-empty code. A routing 404 (e.g. wrong
// URL, nginx default page) typically returns HTML or a body without the code field.
// The caller is responsible for closing resp.Body.
func isMidazAccountNotFound(resp *netHttp.Response) bool {
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil || len(bodyBytes) == 0 {
		return false
	}

	var errBody midazErrorBody
	if err := json.Unmarshal(bodyBytes, &errBody); err != nil {
		return false
	}

	return strings.TrimSpace(errBody.Code) != ""
}

// httpClientTimeout is the default timeout for HTTP requests to external services.
const httpClientTimeout = 30 * time.Second

// httpMaxIdleConns is the maximum number of idle connections across all hosts.
const httpMaxIdleConns = 100

// httpMaxIdleConnsPerHost is the maximum number of idle connections per host.
const httpMaxIdleConnsPerHost = 10

// httpIdleConnTimeout is the maximum time an idle connection will remain idle before closing.
const httpIdleConnTimeout = 90 * time.Second

type MidazService struct {
	authTokenGetter     AuthTokenGetter
	client              *netHttp.Client
	MidazOnboardingURL  string
	MidazTransactionURL string
	ClientID            string
	ClientSecret        string
	Version             string
}

// NewMidazService returns a new instance of MidazService using the given auth client and configuration.
// The service is initialized with a dedicated HTTP client configured with timeouts and connection pooling
// per Ring standards (no http.DefaultClient usage).
func NewMidazService(ac *middleware.AuthClient, midazOnboardingURL, midazTransactionURL, clientID, clientSecret, version string) *MidazService {
	if ac == nil {
		panic("http.NewMidazService: AuthClient must not be nil")
	}

	return NewMidazServiceWithAuthGetter(
		&authClientAdapter{authClient: ac},
		midazOnboardingURL, midazTransactionURL,
		clientID, clientSecret, version,
	)
}

// NewMidazServiceWithAuthGetter creates a MidazService with a custom AuthTokenGetter.
// This constructor supports tenant-aware auth: in multi-tenant mode the caller
// supplies a TenantAwareAuthGetter that resolves per-tenant M2M credentials
// before obtaining a token.  In single-tenant mode the standard authClientAdapter
// is passed and behavior is unchanged.
func NewMidazServiceWithAuthGetter(authGetter AuthTokenGetter, midazOnboardingURL, midazTransactionURL, clientID, clientSecret, version string) *MidazService {
	if authGetter == nil {
		panic("http.NewMidazServiceWithAuthGetter: AuthTokenGetter must not be nil")
	}

	transport := &netHttp.Transport{
		MaxIdleConns:        httpMaxIdleConns,
		MaxIdleConnsPerHost: httpMaxIdleConnsPerHost,
		IdleConnTimeout:     httpIdleConnTimeout,
		DisableKeepAlives:   false,
	}

	httpClient := &netHttp.Client{
		Transport: transport,
		Timeout:   httpClientTimeout,
	}

	// Normalize base URLs: ensure exactly one trailing slash so endpoint
	// concatenation never produces double-slashes or missing separators.
	return &MidazService{
		authTokenGetter:     authGetter,
		client:              httpClient,
		MidazOnboardingURL:  strings.TrimRight(midazOnboardingURL, "/"),
		MidazTransactionURL: strings.TrimRight(midazTransactionURL, "/"),
		ClientID:            clientID,
		ClientSecret:        clientSecret,
		Version:             version,
	}
}

func (m *MidazService) GetAccountFromMidazByAlias(
	ctx context.Context,
	creditAccount, organizationID, ledgerID string,
) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "external.midaz.find_account_by_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.midaz.credit_account", creditAccount),
		attribute.String("app.request.midaz.organization_id", organizationID),
		attribute.String("app.request.midaz.ledger_id", ledgerID),
	)

	token, errToken := m.authTokenGetter.GetApplicationToken(ctx, m.ClientID, m.ClientSecret)
	if errToken != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get application token", errToken)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to get account on midaz by alias. Err: %v", errToken))

		return pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", creditAccount)
	}

	endpoint := "/organizations/" + organizationID + "/ledgers/" + ledgerID + "/accounts/alias/" + creditAccount
	getMidazAccountURL := m.MidazOnboardingURL + endpoint

	span.SetAttributes(attribute.String("app.request.midaz.url", getMidazAccountURL))

	req, err := netHttp.NewRequestWithContext(ctx, netHttp.MethodGet, getMidazAccountURL, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request to get account on midaz by alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to get account on midaz by alias. url=%s err=%v", getMidazAccountURL, err))

		return pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", creditAccount)
	}

	libOpentelemetry.InjectHTTPContext(ctx, req.Header)

	m.setCommonHeaders(req, token)

	resp, errReq := m.client.Do(req)
	if errReq != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get account on midaz by alias", errReq)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to get account on midaz by alias. url=%s err=%v", getMidazAccountURL, errReq))

		return pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", creditAccount)
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case netHttp.StatusForbidden, netHttp.StatusUnauthorized:
		logMsg := "Forbidden to get account on midaz by alias"
		err := pkg.ValidateBusinessError(constant.ErrForbiddenAccessMidaz, "", creditAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, logMsg, err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. url=%s status=%v", logMsg, getMidazAccountURL, resp.Status))

		return err
	case netHttp.StatusInternalServerError:
		logMsg := "Error to get account on midaz by alias"
		err := pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", creditAccount)

		libOpentelemetry.HandleSpanError(span, logMsg, err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. url=%s status=%v", logMsg, getMidazAccountURL, resp.Status))

		return err
	case netHttp.StatusNotFound:
		if !isMidazAccountNotFound(resp) {
			logMsg := fmt.Sprintf("Midaz route not found (possible misconfigured URL). alias=%s url=%s", creditAccount, getMidazAccountURL)
			err := pkg.ValidateBusinessError(constant.ErrMidazRouteNotFound, "", creditAccount)

			libOpentelemetry.HandleSpanError(span, logMsg, err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. status=%v — check MIDAZ_ONBOARDING_URL configuration", logMsg, resp.Status))

			return err
		}

		logMsg := fmt.Sprintf("Account alias %v not found on midaz", creditAccount)
		err := pkg.ValidateBusinessError(constant.ErrFindAccountOnMidaz, "", creditAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, logMsg, err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. url=%s status=%v", logMsg, getMidazAccountURL, resp.Status))

		return err
	}

	return nil
}

func (m *MidazService) GetAccountDetailsByAlias(
	ctx context.Context,
	organizationID, ledgerID, alias string,
) (*pkg.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "external.midaz.get_account_details_by_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.midaz.alias", alias),
		attribute.String("app.request.midaz.organization_id", organizationID),
		attribute.String("app.request.midaz.ledger_id", ledgerID),
	)

	token, errToken := m.authTokenGetter.GetApplicationToken(ctx, m.ClientID, m.ClientSecret)
	if errToken != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get application token", errToken)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to get account details on midaz by alias. Err: %v", errToken))

		return nil, pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", alias)
	}

	endpoint := "/organizations/" + organizationID + "/ledgers/" + ledgerID + "/accounts/alias/" + alias
	getMidazAccountURL := m.MidazOnboardingURL + endpoint

	span.SetAttributes(attribute.String("app.request.midaz.url", getMidazAccountURL))

	req, err := netHttp.NewRequestWithContext(ctx, netHttp.MethodGet, getMidazAccountURL, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request to get account details on midaz by alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to get account details on midaz by alias. url=%s err=%v", getMidazAccountURL, err))

		return nil, pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", alias)
	}

	libOpentelemetry.InjectHTTPContext(ctx, req.Header)

	m.setCommonHeaders(req, token)

	resp, errReq := m.client.Do(req)
	if errReq != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get account details on midaz by alias", errReq)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to get account details on midaz by alias. url=%s err=%v", getMidazAccountURL, errReq))

		return nil, pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", alias)
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case netHttp.StatusForbidden, netHttp.StatusUnauthorized:
		logMsg := "Forbidden to get account details on midaz by alias"
		bizErr := pkg.ValidateBusinessError(constant.ErrForbiddenAccessMidaz, "", alias)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. url=%s status=%v", logMsg, getMidazAccountURL, resp.Status))

		return nil, bizErr
	case netHttp.StatusInternalServerError:
		logMsg := "Error to get account details on midaz by alias"
		bizErr := pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", alias)

		libOpentelemetry.HandleSpanError(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. url=%s status=%v", logMsg, getMidazAccountURL, resp.Status))

		return nil, bizErr
	case netHttp.StatusNotFound:
		if !isMidazAccountNotFound(resp) {
			logMsg := fmt.Sprintf("Midaz route not found (possible misconfigured URL). alias=%s url=%s", alias, getMidazAccountURL)
			bizErr := pkg.ValidateBusinessError(constant.ErrMidazRouteNotFound, "", alias)

			libOpentelemetry.HandleSpanError(span, logMsg, bizErr)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. status=%v — check MIDAZ_ONBOARDING_URL configuration", logMsg, resp.Status))

			return nil, bizErr
		}

		logMsg := fmt.Sprintf("Account alias %v not found on midaz", alias)
		bizErr := pkg.ValidateBusinessError(constant.ErrFindAccountOnMidaz, "", alias)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. url=%s status=%v", logMsg, getMidazAccountURL, resp.Status))

		return nil, bizErr
	}

	var account pkg.Account
	if errDecode := json.NewDecoder(resp.Body).Decode(&account); errDecode != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode account details response from midaz", errDecode)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to decode account details from midaz response. Err: %v", errDecode))

		return nil, pkg.ValidateBusinessError(constant.ErrFindAccountOnMidaz, "", alias)
	}

	return &account, nil
}

// CountTransactionsByRoute calls the Midaz transaction service to count transactions
// matching a given route, status, and date range.
func (m *MidazService) CountTransactionsByRoute(
	ctx context.Context,
	params CountParams,
) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "midaz.count_transactions")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.midaz.organization_id", params.OrganizationID.String()),
		attribute.String("app.request.midaz.ledger_id", params.LedgerID.String()),
		attribute.String("app.request.midaz.route", params.Route),
		attribute.String("app.request.midaz.status", params.Status),
	)

	token, errToken := m.authTokenGetter.GetApplicationToken(ctx, m.ClientID, m.ClientSecret)
	if errToken != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get application token for count transactions", errToken)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to get application token for count transactions. Err: %v", errToken))

		return 0, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "failed to get auth token")
	}

	endpoint := fmt.Sprintf("/organizations/%s/ledgers/%s/transactions/metrics/count",
		params.OrganizationID.String(), params.LedgerID.String())
	countURL := m.MidazTransactionURL + endpoint

	req, err := netHttp.NewRequestWithContext(ctx, netHttp.MethodHead, countURL, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request for count transactions", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating request for count transactions. Err: %v", err))

		return 0, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "failed to create request")
	}

	q := req.URL.Query()
	q.Set("route", params.Route)
	q.Set("status", params.Status)
	q.Set("start_date", params.StartDate.Format(time.RFC3339))
	q.Set("end_date", params.EndDate.Format(time.RFC3339))
	req.URL.RawQuery = q.Encode()

	libOpentelemetry.InjectHTTPContext(ctx, req.Header)

	m.setCommonHeaders(req, token)

	resp, errReq := m.client.Do(req)
	if errReq != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count transactions on midaz", errReq)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to count transactions on midaz. Err: %v", errReq))

		return 0, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "failed to execute request")
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case netHttp.StatusForbidden, netHttp.StatusUnauthorized:
		logMsg := "Forbidden to count transactions on midaz"
		bizErr := pkg.ValidateBusinessError(constant.ErrForbiddenAccessMidaz, "", "count transactions")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. Midaz Error: %v", logMsg, resp.Status))

		return 0, bizErr
	case netHttp.StatusNotFound:
		logMsg := fmt.Sprintf("Midaz transaction route not found (possible misconfigured URL). url=%s", countURL)
		bizErr := pkg.ValidateBusinessError(constant.ErrMidazRouteNotFound, "", "count transactions")

		libOpentelemetry.HandleSpanError(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. status=%v — check MIDAZ_TRANSACTION_URL configuration", logMsg, resp.Status))

		return 0, bizErr
	case netHttp.StatusInternalServerError:
		logMsg := "Error to count transactions on midaz"
		bizErr := pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", "count transactions")

		libOpentelemetry.HandleSpanError(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. Midaz Error: %v", logMsg, resp.Status))

		return 0, bizErr
	}

	// The count is returned in the X-Total-Count response header (HEAD 204).
	totalCountStr := resp.Header.Get("X-Total-Count")
	if totalCountStr == "" {
		bizErr := pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "missing X-Total-Count header")
		libOpentelemetry.HandleSpanError(span, "Missing X-Total-Count header in count transactions response", bizErr)

		logger.Log(ctx, libLog.LevelError, "Missing X-Total-Count header in count transactions response")

		return 0, bizErr
	}

	totalCount, errParse := strconv.ParseInt(totalCountStr, 10, 64)
	if errParse != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse X-Total-Count header", errParse)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error parsing X-Total-Count header value=%s. Err: %v", totalCountStr, errParse))

		return 0, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "failed to parse X-Total-Count header")
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully counted transactions: totalCount=%d", totalCount))

	return totalCount, nil
}

// ListAccounts calls the Midaz onboarding service to retrieve a paginated list of accounts
// for a given organization and ledger, with optional segment and portfolio filters.
func (m *MidazService) ListAccounts(
	ctx context.Context,
	orgID, ledgerID uuid.UUID,
	filters AccountFilters,
	page, limit int,
) (*AccountPage, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "midaz.list_accounts")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.midaz.organization_id", orgID.String()),
		attribute.String("app.request.midaz.ledger_id", ledgerID.String()),
		attribute.Int("app.request.midaz.page", page),
		attribute.Int("app.request.midaz.limit", limit),
	)

	token, errToken := m.authTokenGetter.GetApplicationToken(ctx, m.ClientID, m.ClientSecret)
	if errToken != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get application token for list accounts", errToken)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to get application token for list accounts. Err: %v", errToken))

		return nil, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "failed to get auth token")
	}

	endpoint := fmt.Sprintf("/organizations/%s/ledgers/%s/accounts",
		orgID.String(), ledgerID.String())
	listURL := m.MidazOnboardingURL + endpoint

	req, err := netHttp.NewRequestWithContext(ctx, netHttp.MethodGet, listURL, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request for list accounts", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating request for list accounts. Err: %v", err))

		return nil, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "failed to create request")
	}

	q := req.URL.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))

	if filters.SegmentID != nil {
		q.Set("segment_id", filters.SegmentID.String())
	}

	if filters.PortfolioID != nil {
		q.Set("portfolio_id", filters.PortfolioID.String())
	}

	req.URL.RawQuery = q.Encode()

	libOpentelemetry.InjectHTTPContext(ctx, req.Header)

	m.setCommonHeaders(req, token)

	resp, errReq := m.client.Do(req)
	if errReq != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list accounts on midaz", errReq)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to list accounts on midaz. Err: %v", errReq))

		return nil, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "failed to execute request")
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case netHttp.StatusForbidden, netHttp.StatusUnauthorized:
		logMsg := "Forbidden to list accounts on midaz"
		bizErr := pkg.ValidateBusinessError(constant.ErrForbiddenAccessMidaz, "", "list accounts")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. Midaz Error: %v", logMsg, resp.Status))

		return nil, bizErr
	case netHttp.StatusNotFound:
		logMsg := fmt.Sprintf("Midaz onboarding route not found (possible misconfigured URL). url=%s", listURL)
		bizErr := pkg.ValidateBusinessError(constant.ErrMidazRouteNotFound, "", "list accounts")

		libOpentelemetry.HandleSpanError(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. status=%v — check MIDAZ_ONBOARDING_URL configuration", logMsg, resp.Status))

		return nil, bizErr
	case netHttp.StatusInternalServerError:
		logMsg := "Error to list accounts on midaz"
		bizErr := pkg.ValidateBusinessError(constant.ErrAccessMidaz, "", "list accounts")

		libOpentelemetry.HandleSpanError(span, logMsg, bizErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("%s. Midaz Error: %v", logMsg, resp.Status))

		return nil, bizErr
	}

	var accountPage AccountPage
	if errDecode := json.NewDecoder(resp.Body).Decode(&accountPage); errDecode != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode list accounts response from midaz", errDecode)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to decode list accounts response from midaz. Err: %v", errDecode))

		return nil, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "failed to decode response")
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully listed accounts: count=%d page=%d", len(accountPage.Items), accountPage.Page))

	return &accountPage, nil
}

// setCommonHeaders sets the Authorization, User-Agent, and Content-Type headers on the request.
func (m *MidazService) setCommonHeaders(req *netHttp.Request, token string) {
	version := m.Version
	if version == "" {
		version = "v1.0.0"
	}

	if len(version) > 0 && version[0] == 'v' {
		version = version[1:]
	}

	userAgent := "plugin-fees/" + version + " LerianStudio"

	bearerToken := "Bearer " + token
	req.Header.Add("Authorization", bearerToken)
	req.Header.Add("User-Agent", userAgent)
}
