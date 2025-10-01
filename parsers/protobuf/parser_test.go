package protobuf_test

import (
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sevigo/goframe/parsers/protobuf"
	"github.com/sevigo/goframe/schema"
)

// mockFileInfo implements fs.FileInfo for testing
type mockFileInfo struct {
	name  string
	isDir bool
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() fs.FileMode  { return 0644 }
func (m mockFileInfo) ModTime() time.Time { return time.Now() }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() interface{}   { return nil }

func TestNewProtobufParser(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	if parser == nil {
		t.Fatal("NewProtobufParser returned nil")
	}
}

func TestProtobufParser_Name(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	if parser.Name() != "protobuf" {
		t.Errorf("Expected name 'protobuf', got '%s'", parser.Name())
	}
}

func TestProtobufParser_Extensions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	extensions := parser.Extensions()
	expected := []string{".proto"}

	if len(extensions) != len(expected) {
		t.Fatalf("Expected %d extensions, got %d", len(expected), len(extensions))
	}

	for i, ext := range extensions {
		if ext != expected[i] {
			t.Errorf("Expected extension '%s', got '%s'", expected[i], ext)
		}
	}
}

func TestProtobufParser_CanHandle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	tests := []struct {
		name     string
		path     string
		isDir    bool
		expected bool
	}{
		{"Valid proto file", "service.proto", false, true},
		{"Valid proto file with path", "api/user.proto", false, true},
		{"Directory", "proto/", true, false},
		{"Test file", "service_test.proto", false, false},
		{"Test file 2", "test_service.proto", false, false},
		{"Test file 3", "service.test.proto", false, false},
		{"Go file", "service.go", false, false},
		{"No extension", "README", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := mockFileInfo{name: tt.path, isDir: tt.isDir}
			result := parser.CanHandle(tt.path, info)
			if result != tt.expected {
				t.Errorf("CanHandle(%s) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestProtobufParser_Chunk_InvalidSyntax(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	invalidProto := `
syntax = "proto3"
message Invalid {
  string name = 1
  // Missing semicolon above
`

	_, err := parser.Chunk(invalidProto, "test.proto", nil)
	if err == nil {
		t.Error("Expected error for invalid syntax, got nil")
	}
}

func TestProtobufParser_Chunk_SimpleMessage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

// User represents a user in the system
message User {
  string name = 1;
  int32 age = 2;
  string email = 3;
}
`

	chunks, err := parser.Chunk(protoContent, "test.proto", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Type != "message" {
		t.Errorf("Expected type 'message', got '%s'", chunk.Type)
	}
	if chunk.Identifier != "User" {
		t.Errorf("Expected identifier 'User', got '%s'", chunk.Identifier)
	}
	if chunk.Annotations["name"] != "User" {
		t.Errorf("Expected annotation name 'User', got '%s'", chunk.Annotations["name"])
	}
	if chunk.Annotations["field_count"] != "3" {
		t.Errorf("Expected field_count '3', got '%s'", chunk.Annotations["field_count"])
	}
	if chunk.Annotations["has_doc"] != "true" {
		t.Errorf("Expected has_doc 'true', got '%s'", chunk.Annotations["has_doc"])
	}
}

func TestProtobufParser_Chunk_NestedMessage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

message Outer {
  string id = 1;
  
  message Inner {
    string value = 1;
  }
  
  Inner inner = 2;
}
`

	chunks, err := parser.Chunk(protoContent, "test.proto", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks, got %d", len(chunks))
	}

	// Check outer message
	if chunks[0].Identifier != "Outer" {
		t.Errorf("Expected first chunk identifier 'Outer', got '%s'", chunks[0].Identifier)
	}

	// Check nested message
	if chunks[1].Identifier != "Outer.Inner" {
		t.Errorf("Expected second chunk identifier 'Outer.Inner', got '%s'", chunks[1].Identifier)
	}
}

func TestProtobufParser_Chunk_Service(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

message Request {
  string query = 1;
}

message Response {
  string result = 1;
}

// UserService provides user management
service UserService {
  // GetUser retrieves a user
  rpc GetUser(Request) returns (Response);
  rpc StreamUsers(Request) returns (stream Response);
}
`

	chunks, err := parser.Chunk(protoContent, "test.proto", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have: Request message, Response message, UserService, GetUser RPC, StreamUsers RPC
	if len(chunks) != 5 {
		t.Fatalf("Expected 5 chunks, got %d", len(chunks))
	}

	// Find service chunk
	var serviceChunk *schema.CodeChunk
	for i := range chunks {
		if chunks[i].Type == "service" {
			serviceChunk = &chunks[i]
			break
		}
	}

	if serviceChunk == nil {
		t.Fatal("Service chunk not found")
	}

	if serviceChunk.Identifier != "UserService" {
		t.Errorf("Expected service identifier 'UserService', got '%s'", serviceChunk.Identifier)
	}
	if serviceChunk.Annotations["rpc_count"] != "2" {
		t.Errorf("Expected rpc_count '2', got '%s'", serviceChunk.Annotations["rpc_count"])
	}

	// Find RPC chunks
	rpcCount := 0
	for _, chunk := range chunks {
		if chunk.Type == "rpc" {
			rpcCount++
			if chunk.Identifier != "UserService.GetUser" && chunk.Identifier != "UserService.StreamUsers" {
				t.Errorf("Unexpected RPC identifier: %s", chunk.Identifier)
			}
		}
	}

	if rpcCount != 2 {
		t.Errorf("Expected 2 RPC chunks, got %d", rpcCount)
	}
}

func TestProtobufParser_Chunk_StreamingRPC(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

message Request {}
message Response {}

service StreamService {
  rpc BiDirectional(stream Request) returns (stream Response);
}
`

	chunks, err := parser.Chunk(protoContent, "test.proto", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Find the RPC chunk
	var rpcChunk *schema.CodeChunk
	for i := range chunks {
		if chunks[i].Type == "rpc" {
			rpcChunk = &chunks[i]
			break
		}
	}

	if rpcChunk == nil {
		t.Fatal("RPC chunk not found")
	}

	if rpcChunk.Annotations["streams_request"] != "true" {
		t.Errorf("Expected streams_request 'true', got '%s'", rpcChunk.Annotations["streams_request"])
	}
	if rpcChunk.Annotations["streams_response"] != "true" {
		t.Errorf("Expected streams_response 'true', got '%s'", rpcChunk.Annotations["streams_response"])
	}
}

func TestProtobufParser_Chunk_Enum(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

// Status represents the status of an operation
enum Status {
  UNKNOWN = 0;
  PENDING = 1;
  COMPLETED = 2;
  FAILED = 3;
}
`

	chunks, err := parser.Chunk(protoContent, "test.proto", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Type != "enum" {
		t.Errorf("Expected type 'enum', got '%s'", chunk.Type)
	}
	if chunk.Identifier != "Status" {
		t.Errorf("Expected identifier 'Status', got '%s'", chunk.Identifier)
	}
	if chunk.Annotations["value_count"] != "4" {
		t.Errorf("Expected value_count '4', got '%s'", chunk.Annotations["value_count"])
	}
}

func TestProtobufParser_Chunk_Oneof(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

message Sample {
  oneof test_oneof {
    string name = 1;
    int32 age = 2;
  }
}
`

	chunks, err := parser.Chunk(protoContent, "test.proto", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have Sample message and test_oneof
	if len(chunks) < 2 {
		t.Fatalf("Expected at least 2 chunks, got %d", len(chunks))
	}

	// Find oneof chunk
	var oneofChunk *schema.CodeChunk
	for i := range chunks {
		if chunks[i].Type == "oneof" {
			oneofChunk = &chunks[i]
			break
		}
	}

	if oneofChunk == nil {
		t.Fatal("Oneof chunk not found")
	}

	if oneofChunk.Identifier != "Sample.test_oneof" {
		t.Errorf("Expected identifier 'Sample.test_oneof', got '%s'", oneofChunk.Identifier)
	}
	if oneofChunk.Annotations["field_count"] != "2" {
		t.Errorf("Expected field_count '2', got '%s'", oneofChunk.Annotations["field_count"])
	}
}

func TestProtobufParser_ExtractMetadata_InvalidSyntax(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	invalidProto := `
syntax = "proto3"
message Invalid {
  string name = 1
`

	_, err := parser.ExtractMetadata(invalidProto, "test.proto")
	if err == nil {
		t.Error("Expected error for invalid syntax, got nil")
	}
}

func TestProtobufParser_ExtractMetadata_Complete(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

package example.v1;

import "google/protobuf/timestamp.proto";
import "other.proto";

// User message
message User {
  string name = 1;
  int32 age = 2;
}

// Status enum
enum Status {
  UNKNOWN = 0;
  ACTIVE = 1;
}

// UserService provides user operations
service UserService {
  rpc GetUser(User) returns (User);
  rpc ListUsers(User) returns (stream User);
}
`

	metadata, err := parser.ExtractMetadata(protoContent, "test.proto")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check language
	if metadata.Language != "protobuf" {
		t.Errorf("Expected language 'protobuf', got '%s'", metadata.Language)
	}

	// Check imports
	if len(metadata.Imports) != 2 {
		t.Errorf("Expected 2 imports, got %d", len(metadata.Imports))
	}

	// Check properties
	if metadata.Properties["package"] != "example.v1" {
		t.Errorf("Expected package 'example.v1', got '%s'", metadata.Properties["package"])
	}
	if metadata.Properties["syntax"] != "proto3" {
		t.Errorf("Expected syntax 'proto3', got '%s'", metadata.Properties["syntax"])
	}
	if metadata.Properties["total_messages"] != "1" {
		t.Errorf("Expected total_messages '1', got '%s'", metadata.Properties["total_messages"])
	}
	if metadata.Properties["total_enums"] != "1" {
		t.Errorf("Expected total_enums '1', got '%s'", metadata.Properties["total_enums"])
	}
	if metadata.Properties["total_services"] != "1" {
		t.Errorf("Expected total_services '1', got '%s'", metadata.Properties["total_services"])
	}
	if metadata.Properties["total_rpcs"] != "2" {
		t.Errorf("Expected total_rpcs '2', got '%s'", metadata.Properties["total_rpcs"])
	}

	// Check definitions
	if len(metadata.Definitions) < 4 {
		t.Errorf("Expected at least 4 definitions, got %d", len(metadata.Definitions))
	}

	// Check symbols
	if len(metadata.Symbols) < 4 {
		t.Errorf("Expected at least 4 symbols, got %d", len(metadata.Symbols))
	}

	// Verify all definitions have proper visibility
	for _, def := range metadata.Definitions {
		if def.Visibility != "public" {
			t.Errorf("Expected visibility 'public' for %s, got '%s'", def.Name, def.Visibility)
		}
	}

	// Verify all symbols are exported
	for _, symbol := range metadata.Symbols {
		if !symbol.IsExport {
			t.Errorf("Expected symbol %s to be exported", symbol.Name)
		}
	}
}

func TestProtobufParser_ExtractMetadata_WithDocumentation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

// User represents a user account
// This is a multi-line comment
message User {
  string name = 1;
}
`

	metadata, err := parser.ExtractMetadata(protoContent, "test.proto")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(metadata.Definitions) != 1 {
		t.Fatalf("Expected 1 definition, got %d", len(metadata.Definitions))
	}

	def := metadata.Definitions[0]
	if def.Documentation == "" {
		t.Error("Expected documentation to be extracted")
	}
	if !strings.Contains(def.Documentation, "User represents a user account") {
		t.Errorf("Documentation doesn't contain expected text: %s", def.Documentation)
	}
}

func TestProtobufParser_ExtractMetadata_Proto2(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto2";

message User {
  required string name = 1;
  optional int32 age = 2;
}
`

	metadata, err := parser.ExtractMetadata(protoContent, "test.proto")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if metadata.Properties["syntax"] != "proto2" {
		t.Errorf("Expected syntax 'proto2', got '%s'", metadata.Properties["syntax"])
	}
}

func TestProtobufParser_LineNumbers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	parser := protobuf.NewProtobufParser(logger)

	protoContent := `syntax = "proto3";

message User {
  string name = 1;
  int32 age = 2;
}

message Address {
  string street = 1;
}
`

	chunks, err := parser.Chunk(protoContent, "test.proto", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks, got %d", len(chunks))
	}

	// First message should start at line 3
	if chunks[0].LineStart != 3 {
		t.Errorf("Expected first chunk to start at line 3, got %d", chunks[0].LineStart)
	}

	// Second message should start at line 8
	if chunks[1].LineStart != 8 {
		t.Errorf("Expected second chunk to start at line 8, got %d", chunks[1].LineStart)
	}

	// Verify line numbers are consistent
	for i, chunk := range chunks {
		if chunk.LineStart > chunk.LineEnd {
			t.Errorf("Chunk %d: LineStart (%d) > LineEnd (%d)", i, chunk.LineStart, chunk.LineEnd)
		}
		if chunk.LineStart < 1 {
			t.Errorf("Chunk %d: Invalid LineStart (%d)", i, chunk.LineStart)
		}
	}
}
