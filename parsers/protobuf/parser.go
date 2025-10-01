package protobuf

import (
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sevigo/goframe/schema"
	"github.com/yoheimuta/go-protoparser/v4"
	"github.com/yoheimuta/go-protoparser/v4/parser"
)

// NewProtobufParser is the factory function that creates a new Protobuf parser plugin.
func NewProtobufParser(logger *slog.Logger) schema.ParserPlugin {
	return &ProtobufParser{
		logger: logger,
	}
}

// ProtobufParser implements the ParserPlugin interface for Protocol Buffer files.
type ProtobufParser struct {
	logger *slog.Logger
}

// Name returns the language name.
func (p *ProtobufParser) Name() string {
	return "protobuf"
}

// Extensions returns the file extensions this parser handles.
func (p *ProtobufParser) Extensions() []string {
	return []string{".proto"}
}

// CanHandle determines if the parser should process a given file.
func (p *ProtobufParser) CanHandle(path string, info fs.FileInfo) bool {
	// Reject directories
	if info.IsDir() {
		return false
	}

	// Check if extension matches
	ext := filepath.Ext(path)
	validExt := false
	for _, e := range p.Extensions() {
		if ext == e {
			validExt = true
			break
		}
	}
	if !validExt {
		return false
	}

	// Reject common test file patterns
	baseName := filepath.Base(path)
	testPatterns := []string{
		"_test.proto",
		".test.proto",
		"test_",
	}
	for _, pattern := range testPatterns {
		if strings.Contains(baseName, pattern) {
			return false
		}
	}

	return true
}

// Chunk breaks down the source content into logical code blocks.
func (p *ProtobufParser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	reader := strings.NewReader(content)
	parsed, err := protoparser.Parse(reader,
		protoparser.WithDebug(false),
		protoparser.WithPermissive(false),
		protoparser.WithFilename(path),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse protobuf file: %w", err)
	}

	var chunks []schema.CodeChunk
	lines := strings.Split(content, "\n")

	// Process all proto body elements
	for _, element := range parsed.ProtoBody {
		switch v := element.(type) {
		case *parser.Message:
			chunks = append(chunks, p.processMessage(v, lines, "")...)
		case *parser.Service:
			chunks = append(chunks, p.processService(v, lines)...)
		case *parser.Enum:
			chunks = append(chunks, p.processEnum(v, lines, "")...)
		}
	}

	return chunks, nil
}

// processMessage handles message definitions and nested elements.
func (p *ProtobufParser) processMessage(msg *parser.Message, lines []string, parentName string) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	messageName := msg.MessageName
	if parentName != "" {
		messageName = parentName + "." + messageName
	}

	// Extract message content
	content := p.extractContent(lines, msg.Meta.Pos.Line, msg.Meta.LastPos.Line)
	hasDoc := len(msg.Comments) > 0

	annotations := map[string]string{
		"name":    msg.MessageName,
		"type":    "message",
		"has_doc": strconv.FormatBool(hasDoc),
	}

	// Count fields
	fieldCount := 0
	for _, element := range msg.MessageBody {
		if _, ok := element.(*parser.Field); ok {
			fieldCount++
		}
	}
	annotations["field_count"] = strconv.Itoa(fieldCount)

	chunk := schema.CodeChunk{
		Content:     content,
		LineStart:   msg.Meta.Pos.Line,
		LineEnd:     msg.Meta.LastPos.Line,
		Type:        "message",
		Identifier:  messageName,
		Annotations: annotations,
	}
	chunks = append(chunks, chunk)

	// Process nested messages and enums
	for _, element := range msg.MessageBody {
		switch nested := element.(type) {
		case *parser.Message:
			chunks = append(chunks, p.processMessage(nested, lines, messageName)...)
		case *parser.Enum:
			chunks = append(chunks, p.processEnum(nested, lines, messageName)...)
		case *parser.Oneof:
			chunks = append(chunks, p.processOneof(nested, lines, messageName)...)
		}
	}

	return chunks
}

// processService handles service definitions.
func (p *ProtobufParser) processService(svc *parser.Service, lines []string) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	// Extract service content
	content := p.extractContent(lines, svc.Meta.Pos.Line, svc.Meta.LastPos.Line)
	hasDoc := len(svc.Comments) > 0

	rpcCount := 0
	for _, element := range svc.ServiceBody {
		if _, ok := element.(*parser.RPC); ok {
			rpcCount++
		}
	}

	annotations := map[string]string{
		"name":      svc.ServiceName,
		"type":      "service",
		"has_doc":   strconv.FormatBool(hasDoc),
		"rpc_count": strconv.Itoa(rpcCount),
	}

	chunk := schema.CodeChunk{
		Content:     content,
		LineStart:   svc.Meta.Pos.Line,
		LineEnd:     svc.Meta.LastPos.Line,
		Type:        "service",
		Identifier:  svc.ServiceName,
		Annotations: annotations,
	}
	chunks = append(chunks, chunk)

	// Process RPCs
	for _, element := range svc.ServiceBody {
		if rpc, ok := element.(*parser.RPC); ok {
			chunks = append(chunks, p.processRPC(rpc, lines, svc.ServiceName)...)
		}
	}

	return chunks
}

// processRPC handles RPC method definitions.
func (p *ProtobufParser) processRPC(rpc *parser.RPC, lines []string, serviceName string) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	identifier := serviceName + "." + rpc.RPCName
	content := p.extractContent(lines, rpc.Meta.Pos.Line, rpc.Meta.LastPos.Line)
	hasDoc := len(rpc.Comments) > 0

	// Build signature
	signature := fmt.Sprintf("rpc %s(%s%s) returns (%s%s)",
		rpc.RPCName,
		p.streamPrefix(rpc.RPCRequest.IsStream),
		rpc.RPCRequest.MessageType,
		p.streamPrefix(rpc.RPCResponse.IsStream),
		rpc.RPCResponse.MessageType,
	)

	annotations := map[string]string{
		"name":             rpc.RPCName,
		"type":             "rpc",
		"has_doc":          strconv.FormatBool(hasDoc),
		"signature":        signature,
		"request_type":     rpc.RPCRequest.MessageType,
		"response_type":    rpc.RPCResponse.MessageType,
		"streams_request":  strconv.FormatBool(rpc.RPCRequest.IsStream),
		"streams_response": strconv.FormatBool(rpc.RPCResponse.IsStream),
	}

	chunk := schema.CodeChunk{
		Content:     content,
		LineStart:   rpc.Meta.Pos.Line,
		LineEnd:     rpc.Meta.LastPos.Line,
		Type:        "rpc",
		Identifier:  identifier,
		Annotations: annotations,
	}
	chunks = append(chunks, chunk)

	return chunks
}

// processEnum handles enum definitions.
func (p *ProtobufParser) processEnum(enum *parser.Enum, lines []string, parentName string) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	enumName := enum.EnumName
	if parentName != "" {
		enumName = parentName + "." + enumName
	}

	content := p.extractContent(lines, enum.Meta.Pos.Line, enum.Meta.LastPos.Line)
	hasDoc := len(enum.Comments) > 0

	// Count enum fields
	fieldCount := 0
	for _, element := range enum.EnumBody {
		if _, ok := element.(*parser.EnumField); ok {
			fieldCount++
		}
	}

	annotations := map[string]string{
		"name":        enum.EnumName,
		"type":        "enum",
		"has_doc":     strconv.FormatBool(hasDoc),
		"value_count": strconv.Itoa(fieldCount),
	}

	chunk := schema.CodeChunk{
		Content:     content,
		LineStart:   enum.Meta.Pos.Line,
		LineEnd:     enum.Meta.LastPos.Line,
		Type:        "enum",
		Identifier:  enumName,
		Annotations: annotations,
	}
	chunks = append(chunks, chunk)

	return chunks
}

// processOneof handles oneof definitions.
func (p *ProtobufParser) processOneof(oneof *parser.Oneof, lines []string, parentName string) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	oneofName := parentName + "." + oneof.OneofName
	content := p.extractContent(lines, oneof.Meta.Pos.Line, oneof.Meta.LastPos.Line)
	hasDoc := len(oneof.Comments) > 0

	fieldCount := len(oneof.OneofFields)

	annotations := map[string]string{
		"name":        oneof.OneofName,
		"type":        "oneof",
		"has_doc":     strconv.FormatBool(hasDoc),
		"field_count": strconv.Itoa(fieldCount),
	}

	chunk := schema.CodeChunk{
		Content:     content,
		LineStart:   oneof.Meta.Pos.Line,
		LineEnd:     oneof.Meta.LastPos.Line,
		Type:        "oneof",
		Identifier:  oneofName,
		Annotations: annotations,
	}
	chunks = append(chunks, chunk)

	return chunks
}

// extractContent extracts the content from lines between start and end (inclusive).
func (p *ProtobufParser) extractContent(lines []string, start, end int) string {
	if start < 1 || end > len(lines) || start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

// streamPrefix returns "stream " if streaming is enabled.
func (p *ProtobufParser) streamPrefix(isStream bool) string {
	if isStream {
		return "stream "
	}
	return ""
}

// ExtractMetadata gathers file-level metadata.
func (p *ProtobufParser) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
	reader := strings.NewReader(content)
	parsed, err := protoparser.Parse(reader,
		protoparser.WithDebug(false),
		protoparser.WithPermissive(false),
		protoparser.WithFilename(path),
	)
	if err != nil {
		return schema.FileMetadata{}, fmt.Errorf("failed to parse protobuf file: %w", err)
	}

	metadata := schema.FileMetadata{
		Language:    "protobuf",
		Imports:     []string{},
		Definitions: []schema.CodeEntityDefinition{},
		Symbols:     []schema.CodeSymbol{},
		Properties:  map[string]string{},
	}

	lines := strings.Split(content, "\n")

	// Counters for properties
	messageCount := 0
	serviceCount := 0
	enumCount := 0
	rpcCount := 0

	// Extract syntax if present
	if parsed.Syntax != nil {
		metadata.Properties["syntax"] = parsed.Syntax.ProtobufVersion
	}

	// Process proto body elements
	for _, element := range parsed.ProtoBody {
		switch v := element.(type) {
		case *parser.Import:
			metadata.Imports = append(metadata.Imports, v.Location)

		case *parser.Package:
			metadata.Properties["package"] = v.Name

		case *parser.Message:
			messageCount++
			p.extractMessageMetadata(v, lines, "", &metadata)

		case *parser.Service:
			serviceCount++
			p.extractServiceMetadata(v, lines, &metadata, &rpcCount)

		case *parser.Enum:
			enumCount++
			p.extractEnumMetadata(v, lines, "", &metadata)
		}
	}

	// Set summary properties
	metadata.Properties["total_messages"] = strconv.Itoa(messageCount)
	metadata.Properties["total_services"] = strconv.Itoa(serviceCount)
	metadata.Properties["total_enums"] = strconv.Itoa(enumCount)
	metadata.Properties["total_rpcs"] = strconv.Itoa(rpcCount)

	return metadata, nil
}

// extractMessageMetadata extracts metadata for a message definition.
func (p *ProtobufParser) extractMessageMetadata(msg *parser.Message, lines []string, parentName string, metadata *schema.FileMetadata) {
	messageName := msg.MessageName
	if parentName != "" {
		messageName = parentName + "." + messageName
	}

	doc := p.extractDocumentation(msg.Comments)
	content := p.extractContent(lines, msg.Meta.Pos.Line, msg.Meta.LastPos.Line)

	def := schema.CodeEntityDefinition{
		Type:          "message",
		Name:          messageName,
		LineStart:     msg.Meta.Pos.Line,
		LineEnd:       msg.Meta.LastPos.Line,
		Visibility:    "public",
		Documentation: doc,
		Signature:     p.getFirstLine(content),
	}
	metadata.Definitions = append(metadata.Definitions, def)

	symbol := schema.CodeSymbol{
		Name:     messageName,
		Type:     "message",
		IsExport: true,
	}
	metadata.Symbols = append(metadata.Symbols, symbol)

	// Process nested elements
	for _, element := range msg.MessageBody {
		switch nested := element.(type) {
		case *parser.Message:
			p.extractMessageMetadata(nested, lines, messageName, metadata)
		case *parser.Enum:
			p.extractEnumMetadata(nested, lines, messageName, metadata)
		}
	}
}

// extractServiceMetadata extracts metadata for a service definition.
func (p *ProtobufParser) extractServiceMetadata(svc *parser.Service, lines []string, metadata *schema.FileMetadata, rpcCount *int) {
	doc := p.extractDocumentation(svc.Comments)
	content := p.extractContent(lines, svc.Meta.Pos.Line, svc.Meta.LastPos.Line)

	def := schema.CodeEntityDefinition{
		Type:          "service",
		Name:          svc.ServiceName,
		LineStart:     svc.Meta.Pos.Line,
		LineEnd:       svc.Meta.LastPos.Line,
		Visibility:    "public",
		Documentation: doc,
		Signature:     p.getFirstLine(content),
	}
	metadata.Definitions = append(metadata.Definitions, def)

	symbol := schema.CodeSymbol{
		Name:     svc.ServiceName,
		Type:     "service",
		IsExport: true,
	}
	metadata.Symbols = append(metadata.Symbols, symbol)

	// Process RPCs
	for _, element := range svc.ServiceBody {
		if rpc, ok := element.(*parser.RPC); ok {
			*rpcCount++
			p.extractRPCMetadata(rpc, lines, svc.ServiceName, metadata)
		}
	}
}

// extractRPCMetadata extracts metadata for an RPC definition.
func (p *ProtobufParser) extractRPCMetadata(rpc *parser.RPC, lines []string, serviceName string, metadata *schema.FileMetadata) {
	rpcName := serviceName + "." + rpc.RPCName
	doc := p.extractDocumentation(rpc.Comments)

	signature := fmt.Sprintf("rpc %s(%s%s) returns (%s%s)",
		rpc.RPCName,
		p.streamPrefix(rpc.RPCRequest.IsStream),
		rpc.RPCRequest.MessageType,
		p.streamPrefix(rpc.RPCResponse.IsStream),
		rpc.RPCResponse.MessageType,
	)

	def := schema.CodeEntityDefinition{
		Type:          "rpc",
		Name:          rpcName,
		LineStart:     rpc.Meta.Pos.Line,
		LineEnd:       rpc.Meta.LastPos.Line,
		Visibility:    "public",
		Documentation: doc,
		Signature:     signature,
	}
	metadata.Definitions = append(metadata.Definitions, def)

	symbol := schema.CodeSymbol{
		Name:     rpcName,
		Type:     "rpc",
		IsExport: true,
	}
	metadata.Symbols = append(metadata.Symbols, symbol)
}

// extractEnumMetadata extracts metadata for an enum definition.
func (p *ProtobufParser) extractEnumMetadata(enum *parser.Enum, lines []string, parentName string, metadata *schema.FileMetadata) {
	enumName := enum.EnumName
	if parentName != "" {
		enumName = parentName + "." + enumName
	}

	doc := p.extractDocumentation(enum.Comments)
	content := p.extractContent(lines, enum.Meta.Pos.Line, enum.Meta.LastPos.Line)

	def := schema.CodeEntityDefinition{
		Type:          "enum",
		Name:          enumName,
		LineStart:     enum.Meta.Pos.Line,
		LineEnd:       enum.Meta.LastPos.Line,
		Visibility:    "public",
		Documentation: doc,
		Signature:     p.getFirstLine(content),
	}
	metadata.Definitions = append(metadata.Definitions, def)

	symbol := schema.CodeSymbol{
		Name:     enumName,
		Type:     "enum",
		IsExport: true,
	}
	metadata.Symbols = append(metadata.Symbols, symbol)
}

// extractDocumentation extracts documentation from comments.
func (p *ProtobufParser) extractDocumentation(comments []*parser.Comment) string {
	if len(comments) == 0 {
		return ""
	}

	var docs []string
	for _, comment := range comments {
		docs = append(docs, strings.TrimSpace(comment.Raw))
	}
	return strings.Join(docs, "\n")
}

// getFirstLine returns the first non-empty line from content.
func (p *ProtobufParser) getFirstLine(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
