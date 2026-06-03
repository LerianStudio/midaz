// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"bytes"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// QueryHeader entity from query parameter from get apis
type QueryHeader struct {
	Metadata     *bson.M
	OutputFormat string
	Description  string
	Status       string
	TemplateID   uuid.UUID
	Limit        int
	Page         int
	Cursor       string
	SortOrder    string
	CreatedAt    time.Time
	Alias        string
	UseMetadata  bool
	ToAssetCodes []string
	Active       *bool
	Type         string
	StartDate    time.Time
	EndDate      time.Time
}

// Pagination entity from query parameter from get apis
type Pagination struct {
	Limit     int
	Page      int
	Cursor    string
	SortOrder string
	Alias     string
}

func (qh *QueryHeader) ToOffsetPagination() Pagination {
	return Pagination{
		Limit:     qh.Limit,
		Page:      qh.Page,
		SortOrder: qh.SortOrder,
		Alias:     qh.Alias,
	}
}

// normalizeParams rewrites legacy camelCase query parameter keys to their
// snake_case equivalents so the parsing loop only needs to match one format.
// When both formats are present for the same parameter, snake_case takes precedence.
func normalizeParams(params map[string]string) map[string]string {
	aliases := map[string]string{
		"outputFormat": "output_format",
		"sortOrder":    "sort_order",
		"templateId":   "template_id",
		"createdAt":    "created_at",
		"startDate":    "start_date",
		"endDate":      "end_date",
	}

	normalized := make(map[string]string, len(params))

	for k, v := range params {
		normalized[k] = v
	}

	for camel, snake := range aliases {
		if _, hasSnake := normalized[snake]; hasSnake {
			// snake_case already present; remove legacy camelCase if it exists
			delete(normalized, camel)
			continue
		}

		if val, hasCamel := normalized[camel]; hasCamel {
			normalized[snake] = val
			delete(normalized, camel)
		}
	}

	return normalized
}

// parsePositiveInt parses a string as an integer and validates that the result
// is at least 1. It returns a validation error referencing paramName on failure.
func parsePositiveInt(value, paramName string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", paramName)
	}

	if parsed < 1 {
		return 0, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", paramName)
	}

	return parsed, nil
}

// parseFilterParams extracts filter-related query parameters (active, type, date ranges)
// into the provided QueryHeader. This is separated from pagination/format parsing
// to keep cyclomatic complexity manageable.
func parseFilterParams(qh *QueryHeader, key, value string) error {
	switch key {
	case "description":
		qh.Description = value
	case "status":
		qh.Status = value
	case "created_at":
		parsed, errParse := time.Parse("2006-01-02", value)
		if errParse != nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "created_at")
		}

		qh.CreatedAt = parsed
	case "active":
		switch strings.ToLower(value) {
		case "true":
			b := true
			qh.Active = &b
		case "false":
			b := false
			qh.Active = &b
		default:
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "active")
		}
	case "type":
		qh.Type = value
	case "start_date":
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "start_date")
		}

		qh.StartDate = parsed
	case "end_date":
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "end_date")
		}

		qh.EndDate = parsed
	default:
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", key)
	}

	return nil
}

// ValidateParameters validate and return struct of default parameters.
// It accepts both snake_case (preferred) and camelCase (deprecated) query parameter names.
func ValidateParameters(params map[string]string) (*QueryHeader, error) {
	params = normalizeParams(params)

	qh := &QueryHeader{
		Limit:     constant.DefaultPaginationLimit,
		Page:      constant.DefaultPaginationPage,
		SortOrder: "desc",
	}

	for key, value := range params {
		switch {
		case strings.HasPrefix(key, "metadata."):
			if qh.Metadata == nil {
				qh.Metadata = &bson.M{}
			}

			(*qh.Metadata)[key] = value
			qh.UseMetadata = true
		case key == "output_format":
			if !pkg.IsOutputFormatValuesValid(&value) {
				return nil, pkg.ValidateBusinessError(constant.ErrInvalidOutputFormat, "")
			}

			qh.OutputFormat = value
		case key == "template_id":
			parsedID, err := uuid.Parse(value)
			if err != nil {
				return nil, pkg.ValidateBusinessError(constant.ErrInvalidTemplateID, "")
			}

			qh.TemplateID = parsedID
		case key == "limit":
			parsed, err := parsePositiveInt(value, "limit")
			if err != nil {
				return nil, err
			}

			qh.Limit = parsed
		case key == "page":
			parsed, err := parsePositiveInt(value, "page")
			if err != nil {
				return nil, err
			}

			qh.Page = parsed
		case key == "cursor":
			qh.Cursor = value
		case key == "sort_order":
			qh.SortOrder = strings.ToLower(value)
		default:
			if errFilter := parseFilterParams(qh, key, value); errFilter != nil {
				return nil, errFilter
			}
		}
	}

	err := validatePagination(qh.Cursor, qh.SortOrder, qh.Limit)
	if err != nil {
		return nil, err
	}

	return qh, nil
}

// GetFileFromHeader method that get file from header and give a string
func GetFileFromHeader(fileHeader *multipart.FileHeader) (string, error) {
	if !strings.Contains(fileHeader.Filename, fileExtension) {
		return "", pkg.ValidateBusinessError(constant.ErrInvalidFileFormat, "")
	}

	if fileHeader.Size == 0 {
		return "", pkg.ValidateBusinessError(constant.ErrEmptyFile, "")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}

	defer func(file multipart.File) {
		_ = file.Close()
	}(file)

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return "", pkg.ValidateBusinessError(constant.ErrInvalidFileUploaded, "", err)
	}

	fileString := buf.String()

	return fileString, nil
}

func ReadMultipartFile(fileHeader *multipart.FileHeader) ([]byte, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}

func validatePagination(cursor, sortOrder string, limit int) error {
	maxPaginationLimit := pkg.SafeInt64ToInt(pkg.GetenvIntOrDefault("MAX_PAGINATION_LIMIT", constant.DefaultMaxPaginationLimit))

	if limit > maxPaginationLimit {
		return pkg.ValidateBusinessError(constant.ErrPaginationLimitExceeded, "", maxPaginationLimit)
	}

	if (sortOrder != string(constant.Asc)) && (sortOrder != string(constant.Desc)) {
		return pkg.ValidateBusinessError(constant.ErrInvalidSortOrder, "")
	}

	if !pkg.IsNilOrEmpty(&cursor) {
		_, err := DecodeCursor(cursor)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "cursor")
		}
	}

	return nil
}
