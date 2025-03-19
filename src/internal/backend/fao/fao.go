package fao

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
)

type FAO interface {
    SaveFile(path string, data io.Reader) error
    GetFile(path string) (io.ReadCloser, error)
    DeleteFile(path string) error
    FileExists(filename string) bool
}

type LocalFao struct {
    basePath string
}

func NewLocalFao(basePath string) *LocalFao {
    return &LocalFao{basePath: basePath}
}

// writes a file to disk, path being the destination, data being the stream.
func (l *LocalFao) SaveFile(path string, data io.Reader) error {
    filePath := filepath.Join(l.basePath, path)

    file, err := os.Create(filePath)
    if err != nil {
        return fmt.Errorf("failed to create file: %w", err)
    }
    defer file.Close()

    _, err = io.Copy(file, data)
    if err != nil {
        return fmt.Errorf("failed to write file: %w", err)
    }

    return nil
}

// retrieves a file from disk, path being the source, returning a stream/error
func (l *LocalFao) GetFile(path string) (io.ReadCloser, error) {
    filePath := filepath.Join(l.basePath, path)

    file, err := os.Open(filePath)
    if err != nil {
        return nil, fmt.Errorf("failed to open file: %w", err)
    }

    return file, nil
}

// deletes a file on disk
func (l *LocalFao) DeleteFile(path string) error {
    filePath := filepath.Join(l.basePath, path)

    err := os.Remove(filePath)
    if err != nil {
        return fmt.Errorf("failed to delete file: %w", err)
    }

    return nil
}
// returns true if file exists, otherwise false.
func (l *LocalFao) FileExists(path string) bool {
    filePath := filepath.Join(l.basePath, path)
    _, err := os.Stat(filePath)
    return !os.IsNotExist(err)
}
