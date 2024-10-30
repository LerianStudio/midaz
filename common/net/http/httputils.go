package http

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/common"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
)

// QueryHeader entity from query parameter from get apis
type QueryHeader struct {
	Metadata    *bson.M
	Limit       int
	Page        int
	UseMetadata bool
	PortfolioID string
}

// ValidateParameters validate and return struct of default parameters
func ValidateParameters(params map[string]string) *QueryHeader {
	var metadata *bson.M

	limit := 10

	page := 1

	useMetadata := false

	var portfolioID string

	for key, value := range params {
		switch {
		case strings.Contains(key, "metadata."):
			metadata = &bson.M{key: value}
			useMetadata = true
		case strings.Contains(key, "limit"):
			limit, _ = strconv.Atoi(value)
		case strings.Contains(key, "page"):
			page, _ = strconv.Atoi(value)
		case strings.Contains(key, "portfolio_id"):
			portfolioID = value
		}
	}

	query := &QueryHeader{
		Metadata:    metadata,
		Limit:       limit,
		Page:        page,
		UseMetadata: useMetadata,
		PortfolioID: portfolioID,
	}

	return query
}

// IPAddrFromRemoteAddr removes port information from string.
func IPAddrFromRemoteAddr(s string) string {
	idx := strings.LastIndex(s, ":")
	if idx == -1 {
		return s
	}

	return s[:idx]
}

// GetRemoteAddress returns IP address of the client making the request.
// It checks for X-Real-Ip or X-Forwarded-For headers which is used by Proxies.
func GetRemoteAddress(r *http.Request) string {
	realIP := r.Header.Get(headerRealIP)
	forwardedFor := r.Header.Get(headerForwardedFor)

	if realIP == "" && forwardedFor == "" {
		return IPAddrFromRemoteAddr(r.RemoteAddr)
	}

	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}

		return parts[0]
	}

	return realIP
}

// GetFileFromHeader method that get file from header and give a string fom this dsl gold file
func GetFileFromHeader(ctx *fiber.Ctx) (string, error) {
	fileHeader, err := ctx.FormFile(dsl)
	if err != nil {
		return "", common.ValidateBusinessError(cn.ErrInvalidDSLFileFormat, "")
	}

	if !strings.Contains(fileHeader.Filename, fileExtension) {
		return "", common.ValidateBusinessError(cn.ErrInvalidDSLFileFormat, "", fileHeader.Filename)
	}

	if fileHeader.Size == 0 {
		return "", common.ValidateBusinessError(cn.ErrEmptyDSLFile, "", fileHeader.Filename)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}

	defer func(file multipart.File) {
		err := file.Close()
		if err != nil {
			panic(0)
		}
	}(file)

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return "", common.ValidateBusinessError(cn.ErrInvalidDSLFileFormat, "", fileHeader.Filename)
	}

	fileString := buf.String()

	return fileString, nil
}

// GetTokenHeader func that get token from header
func GetTokenHeader(c *fiber.Ctx) string {
	splitToken := strings.Split(c.Get(fiber.HeaderAuthorization), "Bearer")
	if len(splitToken) == 2 {
		return strings.TrimSpace(splitToken[1])
	}

	return ""
}
