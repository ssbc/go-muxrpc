package proc

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type StdioConn struct {
	io.ReadCloser
	io.WriteCloser
	waitErr chan error
}

// Close calls Close on both closers
func (s StdioConn) Close() error {
	if err := s.ReadCloser.Close(); err != nil {
		return err
	}
	if err := s.WriteCloser.Close(); err != nil {
		return err
	}
	return <-s.waitErr
}

func StartStdioProcess(path string, stderr io.Writer, args ...string) (*StdioConn, error) {
	var (
		err  error
		cmd  = exec.Command(path, args...)
		conn StdioConn
	)

	conn.ReadCloser, err = cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	conn.WriteCloser, err = cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	conn.waitErr = make(chan error)

	if stderr == nil {
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return nil, err
		}

		go func() {
			s := bufio.NewScanner(stderr)
			for s.Scan() {
				fmt.Fprintln(os.Stderr, "stdioproc err:", s.Text())
			}
			if err := s.Err(); err != nil {
				fmt.Fprintln(os.Stderr, "stdioproc failed:", err)
			}
		}()
	} else {
		cmd.Stderr = stderr
	}

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		conn.waitErr <- cmd.Wait()
	}()

	return &conn, nil
}
