package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type FileSink struct {
	mu      sync.Mutex
	files   map[string]*os.File
	written map[string]int64
	dir     string
	maxSize int64
}

const defaultTaskOutputMaxBytes int64 = 1 << 20

func NewFileSink(dir string) *FileSink {
	return &FileSink{
		files:   make(map[string]*os.File),
		written: make(map[string]int64),
		dir:     dir,
		maxSize: defaultTaskOutputMaxBytes,
	}
}

func (s *FileSink) Append(taskID string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, ok := s.files[taskID]
	if !ok {
		if err := os.MkdirAll(s.dir, 0o755); err != nil {
			return err
		}
		var err error
		file, err = os.OpenFile(filepath.Join(s.dir, taskID+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		s.files[taskID] = file
	}
	if s.maxSize > 0 {
		remaining := s.maxSize - s.written[taskID]
		if remaining <= 0 {
			return fmt.Errorf("task output exceeded %d bytes", s.maxSize)
		}
		if int64(len(data)) > remaining {
			data = data[:remaining]
		}
	}
	n, err := file.Write(data)
	s.written[taskID] += int64(n)
	if err == nil && s.maxSize > 0 && s.written[taskID] >= s.maxSize {
		return fmt.Errorf("task output exceeded %d bytes", s.maxSize)
	}
	return err
}

func (s *FileSink) Close(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, ok := s.files[taskID]
	if !ok {
		return nil
	}
	delete(s.files, taskID)
	delete(s.written, taskID)
	return file.Close()
}
