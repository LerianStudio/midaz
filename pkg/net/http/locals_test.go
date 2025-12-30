package http

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalUUID_ValidUUID(t *testing.T) {
	app := fiber.New()
	expectedID := uuid.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", expectedID)
		result := LocalUUID(c, "organization_id")
		assert.Equal(t, expectedID, result)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalUUID_MissingKey_Panics(t *testing.T) {
	app := fiber.New()

	app.Get("/test/:id", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			// Verify panic message contains expected information
			assert.Contains(t, panicMsg, "assertion failed: middleware must set locals key")
			assert.Contains(t, panicMsg, "key=missing_key")
			assert.Contains(t, panicMsg, "path=/test/123")
			assert.Contains(t, panicMsg, "method=GET")
		}()

		LocalUUID(c, "missing_key")
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("GET", "/test/123", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}

func TestLocalUUID_WrongType_Panics(t *testing.T) {
	tests := []struct {
		name         string
		value        any
		expectedType string
	}{
		{
			name:         "string value",
			value:        "not-a-uuid",
			expectedType: "string",
		},
		{
			name:         "int value",
			value:        12345,
			expectedType: "int",
		},
		{
			name:         "struct value",
			value:        struct{ ID string }{ID: "test"},
			expectedType: "struct { ID string }",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()

			app.Get("/test", func(c *fiber.Ctx) error {
				defer func() {
					r := recover()
					require.NotNil(t, r, "expected panic but none occurred")

					panicMsg, ok := r.(string)
					require.True(t, ok, "panic value should be string")

					// Verify panic message contains expected information
					assert.Contains(t, panicMsg, "assertion failed: locals value must be uuid.UUID")
					assert.Contains(t, panicMsg, "key=org_id")
					assert.Contains(t, panicMsg, "actual_type="+tc.expectedType)
					assert.Contains(t, panicMsg, "path=/test")
				}()

				c.Locals("org_id", tc.value)
				LocalUUID(c, "org_id")
				t.Error("expected panic but function returned normally")
				return nil
			})

			req := httptest.NewRequest("GET", "/test", nil)
			_, err := app.Test(req, -1)
			require.NoError(t, err)
		})
	}
}

func TestLocalUUIDOptional_MissingKey_ReturnsNil(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		result := LocalUUIDOptional(c, "missing_key")
		assert.Equal(t, uuid.Nil, result)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalUUIDOptional_ValidUUID_ReturnsUUID(t *testing.T) {
	app := fiber.New()
	expectedID := uuid.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("parent_id", expectedID)
		result := LocalUUIDOptional(c, "parent_id")
		assert.Equal(t, expectedID, result)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalUUIDOptional_WrongType_Panics(t *testing.T) {
	tests := []struct {
		name         string
		value        any
		expectedType string
	}{
		{
			name:         "string value",
			value:        "not-a-uuid",
			expectedType: "string",
		},
		{
			name:         "int value",
			value:        42,
			expectedType: "int",
		},
		{
			name:         "bool value",
			value:        true,
			expectedType: "bool",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()

			app.Get("/test", func(c *fiber.Ctx) error {
				defer func() {
					r := recover()
					require.NotNil(t, r, "expected panic but none occurred")

					panicMsg, ok := r.(string)
					require.True(t, ok, "panic value should be string")

					// Verify panic message contains expected information
					assert.Contains(t, panicMsg, "assertion failed: locals value must be uuid.UUID when set")
					assert.Contains(t, panicMsg, "key=parent_id")
					assert.Contains(t, panicMsg, "actual_type="+tc.expectedType)
					assert.Contains(t, panicMsg, "path=/test")
				}()

				c.Locals("parent_id", tc.value)
				LocalUUIDOptional(c, "parent_id")
				t.Error("expected panic but function returned normally")
				return nil
			})

			req := httptest.NewRequest("GET", "/test", nil)
			_, err := app.Test(req, -1)
			require.NoError(t, err)
		})
	}
}

// Test struct types for Payload tests
type CreateUserInput struct {
	Name  string
	Email string
}

type UpdateUserInput struct {
	ID   string
	Name string
}

func TestPayload_ValidType_ReturnsPayload(t *testing.T) {
	app := fiber.New()
	expected := &CreateUserInput{Name: "John", Email: "john@example.com"}

	app.Post("/test", func(c *fiber.Ctx) error {
		result := Payload[*CreateUserInput](c, expected)
		assert.Equal(t, expected, result)
		assert.Equal(t, "John", result.Name)
		assert.Equal(t, "john@example.com", result.Email)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestPayload_NilPayload_Panics(t *testing.T) {
	app := fiber.New()

	app.Post("/users", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			// Verify panic message contains expected information
			assert.Contains(t, panicMsg, "assertion failed: payload must not be nil after validation")
			assert.Contains(t, panicMsg, "path=/users")
			assert.Contains(t, panicMsg, "method=POST")
		}()

		Payload[*CreateUserInput](c, nil)
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("POST", "/users", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}

func TestPayload_WrongType_Panics(t *testing.T) {
	app := fiber.New()
	wrongPayload := &UpdateUserInput{ID: "123", Name: "John"}

	app.Post("/users", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			// Verify panic message contains expected information
			assert.Contains(t, panicMsg, "assertion failed: payload has unexpected type")
			assert.Contains(t, panicMsg, "expected_type=*http.CreateUserInput")
			assert.Contains(t, panicMsg, "actual_type=*http.UpdateUserInput")
			assert.Contains(t, panicMsg, "path=/users")
			assert.Contains(t, panicMsg, "method=POST")
		}()

		Payload[*CreateUserInput](c, wrongPayload)
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("POST", "/users", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}

func TestPayload_GenericTypeParameter_WorksForDifferentTypes(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(c *fiber.Ctx) any
		validate func(t *testing.T, c *fiber.Ctx, payload any)
	}{
		{
			name: "pointer to struct",
			setup: func(c *fiber.Ctx) any {
				return &CreateUserInput{Name: "Alice", Email: "alice@example.com"}
			},
			validate: func(t *testing.T, c *fiber.Ctx, payload any) {
				result := Payload[*CreateUserInput](c, payload)
				assert.Equal(t, "Alice", result.Name)
				assert.Equal(t, "alice@example.com", result.Email)
			},
		},
		{
			name: "string type",
			setup: func(c *fiber.Ctx) any {
				return "simple string payload"
			},
			validate: func(t *testing.T, c *fiber.Ctx, payload any) {
				result := Payload[string](c, payload)
				assert.Equal(t, "simple string payload", result)
			},
		},
		{
			name: "slice type",
			setup: func(c *fiber.Ctx) any {
				return []string{"item1", "item2", "item3"}
			},
			validate: func(t *testing.T, c *fiber.Ctx, payload any) {
				result := Payload[[]string](c, payload)
				assert.Equal(t, []string{"item1", "item2", "item3"}, result)
			},
		},
		{
			name: "map type",
			setup: func(c *fiber.Ctx) any {
				return map[string]int{"a": 1, "b": 2}
			},
			validate: func(t *testing.T, c *fiber.Ctx, payload any) {
				result := Payload[map[string]int](c, payload)
				assert.Equal(t, 1, result["a"])
				assert.Equal(t, 2, result["b"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()

			app.Post("/test", func(c *fiber.Ctx) error {
				payload := tc.setup(c)
				tc.validate(t, c, payload)
				return c.SendStatus(fiber.StatusOK)
			})

			req := httptest.NewRequest("POST", "/test", nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		})
	}
}

func TestTypeName(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: "nil",
		},
		{
			name:     "string value",
			input:    "hello",
			expected: "string",
		},
		{
			name:     "int value",
			input:    42,
			expected: "int",
		},
		{
			name:     "uuid value",
			input:    uuid.New(),
			expected: "uuid.UUID",
		},
		{
			name:     "pointer to struct",
			input:    &CreateUserInput{},
			expected: "*http.CreateUserInput",
		},
		{
			name:     "slice value",
			input:    []string{},
			expected: "[]string",
		},
		{
			name:     "map value",
			input:    map[string]int{},
			expected: "map[string]int",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := typeName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestLocalUUID_DifferentHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			app := fiber.New()

			app.Add(method, "/test", func(c *fiber.Ctx) error {
				defer func() {
					r := recover()
					require.NotNil(t, r, "expected panic")

					panicMsg, ok := r.(string)
					require.True(t, ok)
					assert.Contains(t, panicMsg, "method="+method)
				}()

				LocalUUID(c, "missing")
				return nil
			})

			req := httptest.NewRequest(method, "/test", nil)
			_, err := app.Test(req, -1)
			require.NoError(t, err)
		})
	}
}

func TestLocalUUID_StackTraceIncluded(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic")

			panicMsg, ok := r.(string)
			require.True(t, ok)

			// Verify stack trace is included
			assert.Contains(t, panicMsg, "stack trace:")
			assert.True(t, strings.Contains(panicMsg, "goroutine") ||
				strings.Contains(panicMsg, ".go:"),
				"stack trace should contain file references or goroutine info")
		}()

		LocalUUID(c, "missing_key")
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}

func TestPayload_TypedNilPointer_Panics(t *testing.T) {
	app := fiber.New()

	app.Post("/test", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic for typed nil pointer")

			panicMsg, ok := r.(string)
			require.True(t, ok)
			assert.Contains(t, panicMsg, "payload must not be nil")
		}()

		var nilPayload *CreateUserInput = nil
		Payload[*CreateUserInput](c, nilPayload)
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("POST", "/test", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}

func TestLocalStringSlice_ValidSlice(t *testing.T) {
	app := fiber.New()
	expected := []string{"field1", "field2", "field3"}

	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("patchRemove", expected)
		result := LocalStringSlice(c, "patchRemove")
		assert.Equal(t, expected, result)
		assert.Len(t, result, 3)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalStringSlice_EmptySlice(t *testing.T) {
	app := fiber.New()
	expected := []string{}

	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("patchRemove", expected)
		result := LocalStringSlice(c, "patchRemove")
		assert.Equal(t, expected, result)
		assert.Len(t, result, 0)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalStringSlice_MissingKey_Panics(t *testing.T) {
	app := fiber.New()

	app.Get("/test/:id", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			// Verify panic message contains expected information
			assert.Contains(t, panicMsg, "assertion failed: middleware must set locals key")
			assert.Contains(t, panicMsg, "key=missing_slice")
			assert.Contains(t, panicMsg, "path=/test/123")
			assert.Contains(t, panicMsg, "method=GET")
		}()

		LocalStringSlice(c, "missing_slice")
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("GET", "/test/123", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}

func TestLocalStringSlice_WrongType_Panics(t *testing.T) {
	tests := []struct {
		name         string
		value        any
		expectedType string
	}{
		{
			name:         "string value",
			value:        "not-a-slice",
			expectedType: "string",
		},
		{
			name:         "int slice",
			value:        []int{1, 2, 3},
			expectedType: "[]int",
		},
		{
			name:         "interface slice",
			value:        []any{"a", "b"},
			expectedType: "[]interface {}",
		},
		{
			name:         "uuid value",
			value:        uuid.New(),
			expectedType: "uuid.UUID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()

			app.Get("/test", func(c *fiber.Ctx) error {
				defer func() {
					r := recover()
					require.NotNil(t, r, "expected panic but none occurred")

					panicMsg, ok := r.(string)
					require.True(t, ok, "panic value should be string")

					// Verify panic message contains expected information
					assert.Contains(t, panicMsg, "assertion failed: locals value must be []string")
					assert.Contains(t, panicMsg, "key=fields")
					assert.Contains(t, panicMsg, "actual_type="+tc.expectedType)
					assert.Contains(t, panicMsg, "path=/test")
				}()

				c.Locals("fields", tc.value)
				LocalStringSlice(c, "fields")
				t.Error("expected panic but function returned normally")
				return nil
			})

			req := httptest.NewRequest("GET", "/test", nil)
			_, err := app.Test(req, -1)
			require.NoError(t, err)
		})
	}
}

func TestLocalString_ValidParam(t *testing.T) {
	app := fiber.New()

	app.Get("/test/:alias", func(c *fiber.Ctx) error {
		result := LocalString(c, "alias")
		assert.Equal(t, "@person1", result)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test/@person1", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestLocalString_EmptyParam_Panics(t *testing.T) {
	app := fiber.New()

	// Route with optional param that won't be set
	app.Get("/test", func(c *fiber.Ctx) error {
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected panic but none occurred")

			panicMsg, ok := r.(string)
			require.True(t, ok, "panic value should be string")

			assert.Contains(t, panicMsg, "assertion failed: path parameter must not be empty")
			assert.Contains(t, panicMsg, "param=missing_param")
			assert.Contains(t, panicMsg, "path=/test")
			assert.Contains(t, panicMsg, "method=GET")
		}()

		LocalString(c, "missing_param")
		t.Error("expected panic but function returned normally")
		return nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, err := app.Test(req, -1)
	require.NoError(t, err)
}
