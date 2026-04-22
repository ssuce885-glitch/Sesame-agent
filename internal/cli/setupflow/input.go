package setupflow

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func chooseArrowOption(reader *bufio.Reader, w io.Writer, label string, options []string, defaultIndex int) (int, error) {
	if len(options) == 0 {
		return 0, fmt.Errorf("no options provided for %s", label)
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	idx := defaultIndex
	fmt.Fprintf(w, "%s: %s (use ↑/↓ then Enter)\n", label, options[idx])
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		switch b {
		case '\n':
			return idx, nil
		case '\r':
			next, err := reader.Peek(1)
			if err == nil && len(next) == 1 && next[0] == '\n' {
				_, _ = reader.ReadByte()
			}
			return idx, nil
		case 0x1b:
			open, err := reader.ReadByte()
			if err != nil {
				return 0, err
			}
			dir, err := reader.ReadByte()
			if err != nil {
				return 0, err
			}
			if open != '[' {
				continue
			}
			switch dir {
			case 'A':
				idx = (idx - 1 + len(options)) % len(options)
			case 'B':
				idx = (idx + 1) % len(options)
			}
			fmt.Fprintf(w, "%s: %s\n", label, options[idx])
		default:
			// Ignore typed characters and keep current selection.
		}
	}
}

func readTextInput(reader *bufio.Reader, w io.Writer, label, defaultValue string) (string, error) {
	return readLineInput(reader, w, label, strings.TrimSpace(defaultValue), strings.TrimSpace(defaultValue))
}

func readSecretInput(reader *bufio.Reader, w io.Writer, label, defaultValue string) (string, error) {
	display := ""
	defaultValue = strings.TrimSpace(defaultValue)
	if defaultValue != "" {
		display = "your-key"
	}
	return readLineInput(reader, w, label, display, defaultValue)
}

func readLineInput(reader *bufio.Reader, w io.Writer, label, displayValue, defaultValue string) (string, error) {
	for {
		if strings.TrimSpace(displayValue) != "" {
			fmt.Fprintf(w, "%s [%s]: ", label, displayValue)
		} else {
			fmt.Fprintf(w, "%s: ", label)
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		value := strings.TrimSpace(line)
		if value == "" && err == io.EOF && len(line) == 0 {
			return "", io.EOF
		}
		if value == "" {
			value = strings.TrimSpace(defaultValue)
		}
		if value != "" {
			return value, nil
		}
		if err == io.EOF {
			return "", io.EOF
		}
	}
}
