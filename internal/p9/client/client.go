// Package client provides pure 9P client I/O operations for the denote server.
// This package contains NO business logic - only read/write primitives.
package client

import (
	"fmt"
	"io"
	"strings"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// With9P establishes a connection to the denote 9P server and executes fn.
func With9P(fn func(*client.Fsys) error) error {
	ns := client.Namespace()
	if ns == "" {
		return fmt.Errorf("failed to get namespace")
	}

	conn, err := client.DialService("denote")
	if err != nil {
		return fmt.Errorf("failed to connect to denote service: %w", err)
	}
	defer conn.Close()

	root, err := conn.Attach(nil, "denote", "")
	if err != nil {
		return fmt.Errorf("failed to attach: %w", err)
	}
	defer root.Close()

	return fn(root)
}

// WriteFile writes data to a 9P file at the given path.
func WriteFile(f *client.Fsys, path string, data string) error {
	fid, err := f.Open(path, plan9.OWRITE)
	if err != nil {
		return err
	}
	defer fid.Close()

	_, err = fid.Write([]byte(data))
	return err
}

func ReadFile(f *client.Fsys, path string) (string, error) {
	fid, err := f.Open(path, plan9.OREAD)
	if err != nil {
		return "", err
	}
	defer fid.Close()

	var content []byte
	buf := make([]byte, 8192)
	for {
		n, err := fid.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		if n == 0 {
			break
		}
		content = append(content, buf[:n]...)
	}
	return strings.TrimSpace(string(content)), nil
}

func ReadFields(f *client.Fsys, identifier string, fields ...string) (map[string]string, error) {
	result := make(map[string]string)
	for _, field := range fields {
		val, err := ReadFile(f, "n/"+identifier+"/"+field)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", field, err)
		}
		result[field] = val
	}
	return result, nil
}

