package example

import (
	"encoding/json"
	"testing"
)

// TestNestedStructBuilders tests that builders are generated for nested structs with constructor annotations
func TestNestedStructBuilders(t *testing.T) {
	// Test that we can build the deepest nested struct
	database := NewNestedStructExampleConfigDatabaseBuilder().
		Driver("postgres").
		Host("db.example.com").
		Port(5432).
		Name("myapp").
		Build()

	if database.Driver != "postgres" {
		t.Errorf("Expected Driver to be 'postgres', got '%s'", database.Driver)
	}
	if database.Host != "db.example.com" {
		t.Errorf("Expected Host to be 'db.example.com', got '%s'", database.Host)
	}
	if database.Port != 5432 {
		t.Errorf("Expected Port to be 5432, got %d", database.Port)
	}
	if database.Name != "myapp" {
		t.Errorf("Expected Name to be 'myapp', got '%s'", database.Name)
	}
}

// TestNestedStructTypeAliases tests that type aliases work correctly for compatibility
func TestNestedStructTypeAliases(t *testing.T) {
	// Build nested structures using builders
	database := NewNestedStructExampleConfigDatabaseBuilder().
		Driver("postgres").
		Host("db.example.com").
		Port(5432).
		Name("myapp").
		Build()

	config := NewNestedStructExampleConfigBuilder().
		Host("api.example.com").
		Port(8080).
		Timeout(30).
		Database(*database).
		Build()

	// Test that the type alias allows assignment to the main struct
	nested := NewNestedStructExampleBuilder().
		Id(999).
		Name("ConfigApp").
		Config(config). // This should work due to type alias compatibility
		IsActive(true).
		Build()

	// Verify the nested structure was built correctly
	if nested.Id() != 999 {
		t.Errorf("Expected Id to be 999, got %d", nested.Id())
	}
	if nested.Name() != "ConfigApp" {
		t.Errorf("Expected Name to be 'ConfigApp', got '%s'", nested.Name())
	}
	if !nested.IsActive {
		t.Errorf("Expected IsActive to be true, got %v", nested.IsActive)
	}

	// Test nested field access
	if nested.Config.Host != "api.example.com" {
		t.Errorf("Expected Config.Host to be 'api.example.com', got '%s'", nested.Config.Host)
	}
	if nested.Config.Database.Driver != "postgres" {
		t.Errorf("Expected Config.Database.Driver to be 'postgres', got '%s'", nested.Config.Database.Driver)
	}
}

// TestStructTagPreservation tests that struct tags are preserved in type aliases
func TestStructTagPreservation(t *testing.T) {
	// Build a config with struct tags
	database := NewNestedStructExampleConfigDatabaseBuilder().
		Driver("postgres").
		Host("db.example.com").
		Port(5432).
		Name("myapp").
		Build()

	config := NewNestedStructExampleConfigBuilder().
		Host("api.example.com").
		Port(8080).
		Timeout(30).
		Database(*database).
		Build()

	// Test JSON marshaling to verify struct tags work
	jsonData, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("JSON marshaling failed: %v", err)
	}

	// Unmarshal to verify the struct tags produced the expected JSON field names
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("JSON unmarshaling failed: %v", err)
	}

	// Check that struct tags are working (lowercase field names in JSON)
	if _, ok := result["host"]; !ok {
		t.Errorf("Expected JSON field 'host' from struct tag, but not found")
	}
	if _, ok := result["port"]; !ok {
		t.Errorf("Expected JSON field 'port' from struct tag, but not found")
	}
	if _, ok := result["timeout"]; !ok {
		t.Errorf("Expected JSON field 'timeout' from struct tag, but not found")
	}

	// Check nested struct tags (note: field name is "database" due to json tag)
	if database, ok := result["database"].(map[string]interface{}); ok {
		if _, ok := database["driver"]; !ok {
			t.Errorf("Expected JSON field 'driver' from nested struct tag, but not found")
		}
		if _, ok := database["host"]; !ok {
			t.Errorf("Expected JSON field 'host' from nested struct tag, but not found")
		}
	} else {
		t.Errorf("Expected nested database object in JSON")
	}
}

// TestBuilderChainCompleteness tests that all non-optional fields are included in builder chains
func TestBuilderChainCompleteness(t *testing.T) {
	// Test that the builder chain includes all required fields
	// This test will fail to compile if any required fields are missing from the chain

	// Database builder chain: Driver -> Host -> Port -> Name -> Build
	database := NewNestedStructExampleConfigDatabaseBuilder().
		Driver("postgres").
		Host("localhost").
		Port(5432).
		Name("testdb").
		Build()

	// Note: SslMode is optional (marked with //+gob:_) so it's not in the chain
	if database.SslMode != false {
		// SslMode should have zero value since it's not set
		t.Errorf("Expected SslMode to be false (zero value), got %v", database.SslMode)
	}

	// Config builder chain: Host -> Port -> Timeout -> Database -> Build
	config := NewNestedStructExampleConfigBuilder().
		Host("api.example.com").
		Port(8080).
		Timeout(30).
		Database(*database).
		Build()

	if config.Host != "api.example.com" {
		t.Errorf("Expected Host to be 'api.example.com', got '%s'", config.Host)
	}

	// Main struct builder chain: Id -> Name -> Config -> IsActive -> Build
	nested := NewNestedStructExampleBuilder().
		Id(123).
		Name("TestApp").
		Config(config).
		IsActive(true).
		Build()

	if nested.Id() != 123 {
		t.Errorf("Expected Id to be 123, got %d", nested.Id())
	}
}

// TestPointerToStructSupport tests that pointer-to-struct fields work correctly
func TestPointerToStructSupport(t *testing.T) {
	// The Config field in NestedStructExample is *struct{...} (pointer to struct)
	// Test that this works correctly with type aliases

	config := NewNestedStructExampleConfigBuilder().
		Host("test.example.com").
		Port(9000).
		Timeout(60).
		Database(*NewNestedStructExampleConfigDatabaseBuilder().
			Driver("mysql").
			Host("mysql.example.com").
			Port(3306).
			Name("testdb").
			Build()).
		Build()

	nested := NewNestedStructExampleBuilder().
		Id(456).
		Name("PointerTest").
		Config(config). // Assigning to pointer field
		IsActive(false).
		Build()

	// Verify the pointer assignment worked
	if nested.Config == nil {
		t.Fatalf("Expected Config to be non-nil pointer")
	}

	if nested.Config.Host != "test.example.com" {
		t.Errorf("Expected Config.Host to be 'test.example.com', got '%s'", nested.Config.Host)
	}

	if nested.Config.Database.Driver != "mysql" {
		t.Errorf("Expected Config.Database.Driver to be 'mysql', got '%s'", nested.Config.Database.Driver)
	}
}

// TestInlineInterfaceBuilders tests that builders work correctly with inline interfaces
func TestInlineInterfaceBuilders(t *testing.T) {
	// Create a mock implementation of the inline interface
	mockIO := &mockReadCloser{
		data: []byte("test data"),
	}

	// Test building struct S with inline interface
	result := NewSBuilder().
		Identifier("test-id").
		IO(mockIO).
		Build()

	if result.Identifier != "test-id" {
		t.Errorf("Expected Identifier to be 'test-id', got '%s'", result.Identifier)
	}

	if result.IO == nil {
		t.Errorf("Expected IO to be non-nil")
	}

	// Test that the interface methods work
	buffer := make([]byte, 10)
	n, err := result.IO.Read(buffer)
	if err != nil {
		t.Errorf("Unexpected error from IO.Read: %v", err)
	}
	if n == 0 {
		t.Errorf("Expected to read some data, got 0 bytes")
	}

	err = result.IO.Close()
	if err != nil {
		t.Errorf("Unexpected error from IO.Close: %v", err)
	}
}

// TestInlineInterfaceTypeSignature tests that the generated method signature is correct
func TestInlineInterfaceTypeSignature(t *testing.T) {
	// This test verifies that the generated builder method accepts the correct inline interface type
	// We can't easily test the exact signature programmatically, but we can test that it compiles
	// and works correctly with different implementations

	// Test with a different mock implementation
	mockIO2 := &alternativeMockReadCloser{}

	result := NewSBuilder().
		Identifier("test-signature").
		IO(mockIO2).
		Build()

	if result.Identifier != "test-signature" {
		t.Errorf("Expected Identifier to be 'test-signature', got '%s'", result.Identifier)
	}

	// Test that both implementations work with the same interface
	buffer := make([]byte, 5)
	n, err := result.IO.Read(buffer)
	if err != nil {
		t.Errorf("Unexpected error from alternative implementation: %v", err)
	}
	if n != 5 {
		t.Errorf("Expected to read 5 bytes, got %d", n)
	}
	if string(buffer) != "hello" {
		t.Errorf("Expected to read 'hello', got '%s'", string(buffer))
	}
}

// Mock implementations for testing
type mockReadCloser struct {
	data []byte
	pos  int
}

func (m *mockReadCloser) Read(p []byte) (int, error) {
	if m.pos >= len(m.data) {
		return 0, nil
	}
	n := copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
}

// Alternative mock implementation to test interface compatibility
type alternativeMockReadCloser struct{}

func (m *alternativeMockReadCloser) Read(p []byte) (int, error) {
	// Return "hello" for testing
	data := []byte("hello")
	n := copy(p, data)
	return n, nil
}

func (m *alternativeMockReadCloser) Close() error {
	return nil
}
