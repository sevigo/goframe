package terraform_test

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sevigo/goframe/parsers/terraform"
	"github.com/sevigo/goframe/schema"
)

// Test data for various Terraform configurations
const (
	basicTerraformConfig = `
terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = "us-west-2"
}

resource "aws_instance" "web" {
  ami           = "ami-0c02fb55956c7d316"
  instance_type = "t2.micro"
  
  tags = {
    Name = "WebServer"
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}

variable "instance_count" {
  description = "Number of instances to create"
  type        = number
  default     = 1
}

output "instance_id" {
  description = "ID of the EC2 instance"
  value       = aws_instance.web.id
  sensitive   = false
}

module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
  version = "~> 3.0"
  
  name = "my-vpc"
  cidr = "10.0.0.0/16"
}

locals {
  common_tags = {
    Environment = "test"
    Project     = "terraform-parser"
  }
}
`

	emptyTerraformConfig = ``

	invalidTerraformConfig = `
resource "aws_instance" "web" {
  ami = "ami-123"
  # Missing closing brace
`

	complexTerraformConfig = `
terraform {
  required_version = ">= 1.0"
}

provider "aws" {
  region = var.aws_region
  alias  = "us_west"
}

provider "aws" {
  region = "us-east-1"
  alias  = "us_east"
}

resource "aws_instance" "web" {
  count         = var.instance_count
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.instance_type
  
  vpc_security_group_ids = [aws_security_group.web.id]
  subnet_id             = aws_subnet.public.id
  
  user_data = templatefile("${path.module}/userdata.sh", {
    app_name = var.app_name
  })
  
  tags = merge(local.common_tags, {
    Name = "web-${count.index + 1}"
  })
}

resource "aws_security_group" "web" {
  name_prefix = "web-"
  vpc_id      = aws_vpc.main.id
  
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical
  
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-*"]
  }
}

variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "instance_count" {
  description = "Number of instances"
  type        = number
  default     = 2
}

variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "t2.micro"
}

variable "app_name" {
  description = "Application name"
  type        = string
}

output "instance_ids" {
  description = "IDs of EC2 instances"
  value       = aws_instance.web[*].id
}

output "public_ips" {
  description = "Public IP addresses"
  value       = aws_instance.web[*].public_ip
  sensitive   = true
}

module "database" {
  source = "./modules/database"
  
  vpc_id           = aws_vpc.main.id
  subnet_ids       = aws_subnet.private[*].id
  security_group_id = aws_security_group.db.id
  
  db_name     = var.app_name
  db_username = "admin"
  db_password = var.db_password
}

locals {
  common_tags = {
    Environment = "production"
    Project     = "web-app"
    ManagedBy   = "terraform"
  }
  
  availability_zones = data.aws_availability_zones.available.names
}
`
)

func TestTerraformPlugin_Name(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	expected := "terraform"
	actual := plugin.Name()

	if actual != expected {
		t.Errorf("Expected plugin name %q, got %q", expected, actual)
	}
}

func TestTerraformPlugin_Extensions(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	expected := []string{".tf", ".tfvars", ".hcl"}
	actual := plugin.Extensions()

	if len(actual) != len(expected) {
		t.Errorf("Expected %d extensions, got %d", len(expected), len(actual))
		return
	}

	for i, ext := range expected {
		if actual[i] != ext {
			t.Errorf("Expected extension %q at index %d, got %q", ext, i, actual[i])
		}
	}
}

func TestTerraformPlugin_CanHandle(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	tests := []struct {
		name     string
		path     string
		isDir    bool
		expected bool
	}{
		{"Terraform file", "main.tf", false, true},
		{"Terraform vars file", "variables.tfvars", false, true},
		{"HCL file", "config.hcl", false, true},
		{"Uppercase extension", "main.TF", false, true},
		{"Directory", "terraform", true, false},
		{"Non-terraform file", "main.go", false, false},
		{"No extension", "Dockerfile", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info os.FileInfo
			if tt.isDir {
				info = &mockFileInfo{name: tt.path, isDir: true}
			} else {
				info = &mockFileInfo{name: tt.path, isDir: false}
			}

			actual := plugin.CanHandle(tt.path, info)
			if actual != tt.expected {
				t.Errorf("CanHandle(%q, isDir=%v) = %v, expected %v",
					tt.path, tt.isDir, actual, tt.expected)
			}
		})
	}
}

func TestTerraformPlugin_ExtractMetadata_BasicConfig(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	metadata, err := plugin.ExtractMetadata(basicTerraformConfig, "main.tf")
	if err != nil {
		t.Fatalf("ExtractMetadata failed: %v", err)
	}

	if metadata.Language != "terraform" {
		t.Errorf("Expected language 'terraform', got %q", metadata.Language)
	}

	if metadata.FilePath != "main.tf" {
		t.Errorf("Expected file path 'main.tf', got %q", metadata.FilePath)
	}

	// Test block counts
	expectedCounts := map[string]string{
		"terraform_count": "1",
		"provider_count":  "1",
		"resource_count":  "1",
		"data_count":      "1",
		"variable_count":  "1",
		"output_count":    "1",
		"module_count":    "1",
		"locals_count":    "1",
	}

	for key, expected := range expectedCounts {
		if actual, exists := metadata.Properties[key]; !exists {
			t.Errorf("Expected property %q not found", key)
		} else if actual != expected {
			t.Errorf("Expected %q = %q, got %q", key, expected, actual)
		}
	}

	expectedImports := []string{"terraform-aws-modules/vpc/aws"}
	if len(metadata.Imports) != len(expectedImports) {
		t.Errorf("Expected %d imports, got %d", len(expectedImports), len(metadata.Imports))
	} else {
		for i, expected := range expectedImports {
			if metadata.Imports[i] != expected {
				t.Errorf("Expected import %q, got %q", expected, metadata.Imports[i])
			}
		}
	}

	expectedDefCount := 7
	if len(metadata.Definitions) != expectedDefCount {
		t.Errorf("Expected %d definitions, got %d", expectedDefCount, len(metadata.Definitions))
	}

	expectedSymCount := 9
	if len(metadata.Symbols) < expectedSymCount-2 {
		t.Errorf("Expected at least %d symbols, got %d", expectedSymCount-2, len(metadata.Symbols))
	}

	definitionTypes := make(map[string]int)
	for _, def := range metadata.Definitions {
		definitionTypes[def.Type]++
	}

	expectedDefTypes := map[string]int{
		"provider": 1,
		"resource": 1,
		"data":     1,
		"variable": 1,
		"output":   1,
		"module":   1,
		"locals":   1,
	}

	for defType, expectedCount := range expectedDefTypes {
		if actualCount, exists := definitionTypes[defType]; !exists {
			t.Errorf("Expected definition type %q not found", defType)
		} else if actualCount != expectedCount {
			t.Errorf("Expected %d %q definitions, got %d", expectedCount, defType, actualCount)
		}
	}
}

func TestTerraformPlugin_ExtractMetadata_EmptyConfig(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	metadata, err := plugin.ExtractMetadata(emptyTerraformConfig, "empty.tf")
	if err != nil {
		t.Fatalf("ExtractMetadata failed for empty config: %v", err)
	}

	if len(metadata.Definitions) != 0 {
		t.Errorf("Expected 0 definitions for empty config, got %d", len(metadata.Definitions))
	}

	if len(metadata.Symbols) != 0 {
		t.Errorf("Expected 0 symbols for empty config, got %d", len(metadata.Symbols))
	}

	if len(metadata.Imports) != 0 {
		t.Errorf("Expected 0 imports for empty config, got %d", len(metadata.Imports))
	}
}

func TestTerraformPlugin_ExtractMetadata_InvalidConfig(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	metadata, err := plugin.ExtractMetadata(invalidTerraformConfig, "invalid.tf")

	if err == nil {
		t.Logf("Got expected error for invalid config: %v", err)
	}

	if metadata.Language != "terraform" {
		t.Errorf("Expected language 'terraform' even for invalid config, got %q", metadata.Language)
	}

	if parseErrors, exists := metadata.Properties["parse_errors"]; !exists || parseErrors == "0" {
		t.Error("Expected parse_errors property to be set for invalid config")
	}
}

func TestTerraformPlugin_Chunk_BasicConfig(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	t.Logf("Test config:\n%s", basicTerraformConfig)

	chunks, err := plugin.Chunk(basicTerraformConfig, "main.tf", nil)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	expectedMinChunks := 8
	if len(chunks) < expectedMinChunks {
		t.Errorf("Expected at least %d chunks, got %d", expectedMinChunks, len(chunks))
	}

	chunkTypes := make(map[string]int)
	for _, chunk := range chunks {
		chunkTypes[chunk.Type]++
	}

	expectedTypes := []string{"terraform", "provider", "resource", "data", "variable", "output", "module", "locals"}
	for _, expectedType := range expectedTypes {
		if count, exists := chunkTypes[expectedType]; !exists || count == 0 {
			t.Errorf("Expected chunk type %q not found", expectedType)
		}
	}

	for i, chunk := range chunks {
		if strings.TrimSpace(chunk.Content) == "" {
			t.Errorf("Chunk %d has empty content", i)
		}

		if chunk.LineStart <= 0 {
			t.Errorf("Chunk %d has invalid LineStart: %d", i, chunk.LineStart)
		}

		if chunk.LineEnd < chunk.LineStart {
			t.Errorf("Chunk %d has LineEnd (%d) < LineStart (%d)", i, chunk.LineEnd, chunk.LineStart)
		}

		if chunk.Identifier == "" {
			t.Errorf("Chunk %d has empty identifier", i)
		}

		if chunk.Annotations == nil {
			t.Errorf("Chunk %d has nil annotations", i)
		}
	}
}

func TestTerraformPlugin_Chunk_ComplexConfig(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	chunks, err := plugin.Chunk(complexTerraformConfig, "complex.tf", nil)
	if err != nil {
		t.Fatalf("Chunk failed for complex config: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected chunks for complex config, got none")
	}

	var resourceChunks, providerChunks, variableChunks []schema.CodeChunk
	for _, chunk := range chunks {
		switch chunk.Type {
		case "resource":
			resourceChunks = append(resourceChunks, chunk)
		case "provider":
			providerChunks = append(providerChunks, chunk)
		case "variable":
			variableChunks = append(variableChunks, chunk)
		}
	}

	if len(resourceChunks) < 2 {
		t.Errorf("Expected at least 2 resource chunks, got %d", len(resourceChunks))
	}

	if len(providerChunks) < 2 {
		t.Errorf("Expected at least 2 provider chunks, got %d", len(providerChunks))
	}

	if len(variableChunks) < 3 {
		t.Errorf("Expected at least 3 variable chunks, got %d", len(variableChunks))
	}

	for _, chunk := range resourceChunks {
		if chunk.Annotations["resource_type"] == "" {
			t.Errorf("Resource chunk missing resource_type annotation")
		}
		if chunk.Annotations["resource_name"] == "" {
			t.Errorf("Resource chunk missing resource_name annotation")
		}
	}
}

func TestTerraformPlugin_Chunk_EmptyConfig(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	chunks, err := plugin.Chunk(emptyTerraformConfig, "empty.tf", nil)
	if err != nil {
		t.Fatalf("Chunk failed for empty config: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty config, got %d", len(chunks))
	}
}

func TestTerraformPlugin_Chunk_InvalidConfig(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	chunks, err := plugin.Chunk(invalidTerraformConfig, "invalid.tf", nil)
	if err != nil {
		t.Errorf("Chunk should handle invalid config gracefully, got error: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk for invalid config")
	} else if chunks[0].Annotations["parse_error"] != "true" && chunks[0].Type != "terraform_file" {
		t.Error("Expected parse error indication in chunk annotations")
	}
}

func TestTerraformPlugin_WithNilLogger(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(nil)

	if plugin.Name() != "terraform" {
		t.Error("Plugin should work with nil logger")
	}

	_, err := plugin.ExtractMetadata(basicTerraformConfig, "test.tf")
	if err != nil {
		t.Errorf("Plugin with nil logger should work, got error: %v", err)
	}
}

// Mock file info for testing
type mockFileInfo struct {
	name  string
	isDir bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }

func TestBuildIdentifier(t *testing.T) {
	plugin := terraform.NewTerraformPlugin(slog.Default())

	testConfig := `
resource "aws_instance" "web" {
  ami = "ami-123"
}

data "aws_ami" "ubuntu" {
  most_recent = true
}

module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
}

variable "region" {
  type = string
}

output "instance_id" {
  value = aws_instance.web.id
}
`

	chunks, err := plugin.Chunk(testConfig, "test.tf", nil)
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}

	expectedIdentifiers := map[string]string{
		"resource": "aws_instance.web",
		"data":     "aws_ami.ubuntu",
		"module":   "module.vpc",
		"variable": "variable.region",
		"output":   "output.instance_id",
	}

	actualIdentifiers := make(map[string]string)
	for _, chunk := range chunks {
		actualIdentifiers[chunk.Type] = chunk.Identifier
	}

	for chunkType, expectedID := range expectedIdentifiers {
		if actualID, exists := actualIdentifiers[chunkType]; !exists {
			t.Errorf("Expected %q chunk not found", chunkType)
		} else if actualID != expectedID {
			t.Errorf("Expected %q identifier %q, got %q", chunkType, expectedID, actualID)
		}
	}
}
