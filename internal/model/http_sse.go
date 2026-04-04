package model

import (
	"bufio"
	"io"
	"strings"
)

type sseFrame struct {
	Event string
	Data  string
}

type sseReader struct {
	reader *bufio.Reader
}

func newSSEReader(r io.Reader) *sseReader {
	return &sseReader{reader: bufio.NewReader(r)}
}

func (r *sseReader) Next() (sseFrame, error) {
	var (
		event     string
		dataLines []string
	)

	for {
		line, err := r.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return sseFrame{}, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if event != "" || len(dataLines) > 0 {
				return sseFrame{
					Event: event,
					Data:  strings.Join(dataLines, "\n"),
				}, nil
			}
			if err == io.EOF {
				return sseFrame{}, io.EOF
			}
			continue
		}

		if !strings.HasPrefix(line, ":") {
			field, value, ok := strings.Cut(line, ":")
			if ok {
				value = strings.TrimPrefix(value, " ")
			}
			switch field {
			case "event":
				event = value
			case "data":
				dataLines = append(dataLines, value)
			}
		}

		if err == io.EOF {
			if event != "" || len(dataLines) > 0 {
				return sseFrame{
					Event: event,
					Data:  strings.Join(dataLines, "\n"),
				}, nil
			}
			return sseFrame{}, io.EOF
		}
	}
}
