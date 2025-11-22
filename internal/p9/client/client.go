// Package client provides 9P client helpers for connecting to the denote server.
package client

import (
	"fmt"

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
