// Package transport moves signed supermessage files between nodes. The
// prototype ships the folder transport: "the wire" is a directory, sending is
// an atomic file drop, receiving is reading your own in-folder. The interface
// is what AS2/SFTP/HTTP would implement in production.
package transport

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sender delivers one supermessage file to a destination address.
type Sender interface {
	Send(address, fileName string, b []byte) error
}

// Folder is the local-folder transport. The address is a directory path.
type Folder struct{}

func (Folder) Send(address, fileName string, b []byte) error {
	if err := os.MkdirAll(address, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(address, "."+fileName+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(address, fileName))
}

// Collect drains a node's own transport/in folder, returning each file's
// bytes and removing it. Hidden temp files are skipped.
func Collect(inDir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(inDir)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(inDir, e.Name())
		b, err := os.ReadFile(full)
		if err != nil {
			return out, fmt.Errorf("could not read incoming file %s: %w", e.Name(), err)
		}
		out[e.Name()] = b
		os.Remove(full)
	}
	return out, nil
}
