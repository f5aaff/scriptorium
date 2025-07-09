package converter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"

	"github.com/google/uuid"
)

type Converter interface {
	ConvertFile(inputPath, outputPath, fromFormat, toFormat string) error
	ConvertDocumentByUUID(documentUUID string, fromFormat, toFormat string) (string, error)
	ConvertFileByPath(filePath, fromFormat, toFormat string) (string, error)
	GetAvailableFormats() (map[string][]string, error)
}

type PandocConverter struct {
	pandocPath string
	dao        dao.DAO
	fao        fao.FAO
}

func NewPandocConverter(pandocPath string) *PandocConverter {
	return &PandocConverter{pandocPath: pandocPath}
}

// NewPandocConverterWithInterfaces creates a converter with DAO and FAO interfaces
func NewPandocConverterWithInterfaces(pandocPath string, dao dao.DAO, fao fao.FAO) *PandocConverter {
	return &PandocConverter{
		pandocPath: pandocPath,
		dao:        dao,
		fao:        fao,
	}
}

// run a file through pandoc, returning whatever error the command results in.
func (pc *PandocConverter) runPandoc(inputPath, outputPath, fromFormat, toFormat string) error {
	if pc.pandocPath == "" {
		pc.pandocPath = "pandoc" // Default to system pandoc
	}

	cmd := exec.Command(pc.pandocPath,
		inputPath,
		"-f", fromFormat,
		"-t", toFormat,
		"-o", outputPath)

	return cmd.Run()
}

func (pc *PandocConverter) ConvertFile(inputPath, outputPath, fromFormat, toFormat string) error {
	err := pc.runPandoc(inputPath, outputPath, fromFormat, toFormat)
	if err != nil {
		return err
	}

	return nil
}

// ConvertDocumentByUUID converts a document by its UUID using DAO and FAO
func (pc *PandocConverter) ConvertDocumentByUUID(documentUUID string, fromFormat, toFormat string) (string, error) {
	if pc.dao == nil || pc.fao == nil {
		return "", fmt.Errorf("DAO and FAO interfaces are required for document conversion")
	}

	// Parse UUID
	uuid, err := uuid.Parse(documentUUID)
	if err != nil {
		return "", fmt.Errorf("invalid UUID: %w", err)
	}

	// Get document metadata from DAO
	rawData, err := pc.dao.ReadRaw(uuid)
	if err != nil {
		return "", fmt.Errorf("failed to read document: %w", err)
	}

	// Parse metadata to get file path
	var metadata dao.MetaData
	if err := json.Unmarshal(rawData, &metadata); err != nil {
		return "", fmt.Errorf("failed to parse document metadata: %w", err)
	}

	// Get the file from FAO
	file, err := pc.fao.GetFile(metadata.Path)
	if err != nil {
		return "", fmt.Errorf("failed to get file from storage: %w", err)
	}
	defer file.Close()

	// Create temporary input file
	tempInput, err := os.CreateTemp("", "scriptorium_input_*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer os.Remove(tempInput.Name())
	defer tempInput.Close()

	// Copy file content to temp file
	if _, err := io.Copy(tempInput, file); err != nil {
		return "", fmt.Errorf("failed to copy file to temp: %w", err)
	}

	// Create output filename in same location with new extension
	basePath := strings.TrimSuffix(metadata.Path, filepath.Ext(metadata.Path))
	outputPath := fmt.Sprintf("%s.%s", basePath, toFormat)
	tempOutput, err := os.CreateTemp("", "scriptorium_output_*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp output file: %w", err)
	}
	defer os.Remove(tempOutput.Name())
	defer tempOutput.Close()

	// Convert using pandoc
	if err := pc.runPandoc(tempInput.Name(), tempOutput.Name(), fromFormat, toFormat); err != nil {
		return "", fmt.Errorf("conversion failed: %w", err)
	}

	// Read the converted content
	convertedContent, err := io.ReadAll(tempOutput)
	if err != nil {
		return "", fmt.Errorf("failed to read converted content: %w", err)
	}

	// Save converted file to FAO in same location
	reader := bytes.NewReader(convertedContent)
	if err := pc.fao.SaveFile(outputPath, reader); err != nil {
		return "", fmt.Errorf("failed to save converted file: %w", err)
	}

	return outputPath, nil
}

// ConvertFileByPath converts a file directly from storage using FAO
func (pc *PandocConverter) ConvertFileByPath(filePath, fromFormat, toFormat string) (string, error) {
	if pc.fao == nil {
		return "", fmt.Errorf("FAO interface is required for file conversion")
	}

	// Get the file from FAO
	file, err := pc.fao.GetFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get file from storage: %w", err)
	}
	defer file.Close()

	// Create temporary input file
	tempInput, err := os.CreateTemp("", "scriptorium_input_*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer os.Remove(tempInput.Name())
	defer tempInput.Close()

	// Copy file content to temp file
	if _, err := io.Copy(tempInput, file); err != nil {
		return "", fmt.Errorf("failed to copy file to temp: %w", err)
	}

	// Create output filename in same location with new extension
	basePath := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	outputPath := fmt.Sprintf("%s.%s", basePath, toFormat)
	tempOutput, err := os.CreateTemp("", "scriptorium_output_*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp output file: %w", err)
	}
	defer os.Remove(tempOutput.Name())
	defer tempOutput.Close()

	// Convert using pandoc
	if err := pc.runPandoc(tempInput.Name(), tempOutput.Name(), fromFormat, toFormat); err != nil {
		return "", fmt.Errorf("conversion failed: %w", err)
	}

	// Read the converted content
	convertedContent, err := io.ReadAll(tempOutput)
	if err != nil {
		return "", fmt.Errorf("failed to read converted content: %w", err)
	}

	// Save converted file to FAO in same location
	reader := bytes.NewReader(convertedContent)
	if err := pc.fao.SaveFile(outputPath, reader); err != nil {
		return "", fmt.Errorf("failed to save converted file: %w", err)
	}

	return outputPath, nil
}

// GetAvailableFormats returns supported input and output formats from pandoc
func (pc *PandocConverter) GetAvailableFormats() (map[string][]string, error) {
	// Run pandoc --list-input-formats and --list-output-formats
	inputFormats, err := pc.getPandocFormats("--list-input-formats")
	if err != nil {
		return nil, fmt.Errorf("failed to get input formats: %w", err)
	}

	outputFormats, err := pc.getPandocFormats("--list-output-formats")
	if err != nil {
		return nil, fmt.Errorf("failed to get output formats: %w", err)
	}

	return map[string][]string{
		"input_formats":  inputFormats,
		"output_formats": outputFormats,
	}, nil
}

// getPandocFormats gets the list of supported formats from pandoc
func (pc *PandocConverter) getPandocFormats(formatFlag string) ([]string, error) {
	if pc.pandocPath == "" {
		pc.pandocPath = "pandoc"
	}

	cmd := exec.Command(pc.pandocPath, formatFlag)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get pandoc formats: %w", err)
	}

	// Split output into lines and trim whitespace
	lines := strings.Split(string(output), "\n")
	var formats []string
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			formats = append(formats, trimmed)
		}
	}

	return formats, nil
}
