package setupflow

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func chooseArrowOption(reader *bufio.Reader, w io.Writer, label string, options []string, defaultIndex int) (int, error) {
	if len(options) == 0 {
		return 0, fmt.Errorf("no options provided for %s", label)
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	fmt.Fprintf(w, "%s:\n", label)
	for i, option := range options {
		defaultMarker := ""
		if i == defaultIndex {
			defaultMarker = " [default]"
		}
		fmt.Fprintf(w, "  %d) %s%s\n", i+1, option, defaultMarker)
	}
	for {
		fmt.Fprintf(w, "Select option [%d]: ", defaultIndex+1)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return 0, err
		}
		value := strings.TrimSpace(line)
		if value == "" {
			if err == io.EOF && len(line) == 0 {
				return 0, io.EOF
			}
			return defaultIndex, nil
		}
		selected, convErr := strconv.Atoi(value)
		if convErr == nil && selected >= 1 && selected <= len(options) {
			return selected - 1, nil
		}
		fmt.Fprintf(w, "Invalid selection for %s: enter 1-%d, or press Enter for %d.\n", label, len(options), defaultIndex+1)
		if err == io.EOF {
			return 0, io.EOF
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

func readBoolChoice(reader *bufio.Reader, w io.Writer, label, trueLabel, falseLabel string, defaultValue bool) (bool, error) {
	options := []string{strings.TrimSpace(trueLabel), strings.TrimSpace(falseLabel)}
	defaultIndex := 0
	if !defaultValue {
		defaultIndex = 1
	}
	idx, err := chooseArrowOption(reader, w, label, options, defaultIndex)
	if err != nil {
		return false, err
	}
	return idx == 0, nil
}

func readIntInput(reader *bufio.Reader, w io.Writer, label string, defaultValue int) (int, error) {
	display := ""
	if defaultValue > 0 {
		display = strconv.Itoa(defaultValue)
	}
	for {
		value, err := readLineInput(reader, w, label, display, display)
		if err != nil {
			return 0, err
		}
		n, convErr := strconv.Atoi(strings.TrimSpace(value))
		if convErr == nil {
			return n, nil
		}
		fmt.Fprintf(w, "Invalid number for %s: %s\n", label, value)
	}
}

func parseCommaSeparatedList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
