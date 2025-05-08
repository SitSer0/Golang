//go:build !change

package externalsort

import (
	"bufio"
	"io"
	"strings"
)

type LineReader interface {
	ReadLine() (string, error)
}

type lineReader struct {
	reader *bufio.Reader
}

func (r *lineReader) ReadLine() (string, error) {
	line, err := r.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimRight(line, "\n")
	return line, err
}

type LineWriter interface {
	Write(l string) error
}

type lineWriter struct {
	writer *bufio.Writer
}

func (w *lineWriter) Write(l string) error {
	_, err := w.writer.WriteString(l + "\n")
	if err != nil {
		return err
	}
	return w.writer.Flush()
}
