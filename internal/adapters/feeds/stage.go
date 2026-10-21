package feeds

import (
	"ainfra/internal/domain"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Stage bounds heap usage by grouping the normalized snapshot on disk. The
// temporary directory is private; provider-controlled names are never paths.
type Stage struct {
	dir     string
	files   map[string]string
	open    map[string]*stageWriter
	bytes   int64
	records int
	sealed  bool
}
type stageWriter struct {
	file   *os.File
	writer *bufio.Writer
}

func NewStage(parent string) (*Stage, error) {
	dir, e := os.MkdirTemp(parent, "infra-feed-")
	if e != nil {
		return nil, e
	}
	return &Stage{dir: dir, files: map[string]string{}, open: map[string]*stageWriter{}}, nil
}
func (s *Stage) flush() error {
	var first error
	for key, w := range s.open {
		if e := w.writer.Flush(); e != nil && first == nil {
			first = e
		}
		if e := w.file.Close(); e != nil && first == nil {
			first = e
		}
		delete(s.open, key)
	}
	return first
}
func (s *Stage) Emit(a domain.Advisory) error {
	if s.sealed {
		return errors.New("snapshot sealed")
	}
	if a.Source == "" || len(a.Source) > 300 {
		return errors.New("invalid advisory source")
	}
	b, e := json.Marshal(a)
	if e != nil {
		return e
	}
	if len(b) > 64<<10 {
		return errors.New("normalized advisory too large")
	}
	s.bytes += int64(len(b) + 1)
	s.records++
	if s.bytes > 1<<30 || s.records > 4000000 {
		return errors.New("staging limit exceeded")
	}
	filename, ok := s.files[a.Source]
	if !ok {
		h := sha256.Sum256([]byte(a.Source))
		filename = filepath.Join(s.dir, hex.EncodeToString(h[:]))
		s.files[a.Source] = filename
		if len(s.files) > 20000 {
			return errors.New("source limit exceeded")
		}
	}
	w := s.open[filename]
	if w == nil {
		if len(s.open) >= 64 {
			if e = s.flush(); e != nil {
				return e
			}
		}
		f, e := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		w = &stageWriter{f, bufio.NewWriterSize(f, 16384)}
		s.open[filename] = w
	}
	if _, e = w.writer.Write(b); e != nil {
		return e
	}
	return w.writer.WriteByte('\n')
}
func (s *Stage) Walk(visit func(string, []domain.Advisory) error) error {
	if e := s.flush(); e != nil {
		return e
	}
	s.sealed = true
	if s.records == 0 {
		return errors.New("empty_snapshot")
	}
	sources := make([]string, 0, len(s.files))
	for source := range s.files {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		records, e := s.source(s.files[source])
		if e != nil {
			return e
		}
		if e = visit(source, records); e != nil {
			return e
		}
	}
	return nil
}
func (s *Stage) source(path string) ([]domain.Advisory, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 65537)
	records := map[string]domain.Advisory{}
	for scanner.Scan() {
		var a domain.Advisory
		if e = json.Unmarshal(scanner.Bytes(), &a); e != nil {
			return nil, e
		}
		key := a.ID + "/" + a.Release + "/" + a.Channel
		old, exists := records[key]
		if !exists || strings.Contains(a.NativeID, "CVE-") || !strings.Contains(old.NativeID, "CVE-") {
			records[key] = a
		}
		if len(records) > 100000 {
			return nil, errors.New("per-source record limit exceeded")
		}
	}
	if e = scanner.Err(); e != nil {
		return nil, e
	}
	keys := make([]string, 0, len(records))
	for k := range records {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]domain.Advisory, 0, len(keys))
	for _, k := range keys {
		out = append(out, records[k])
	}
	return out, nil
}
func (s *Stage) Close() error {
	e := s.flush()
	if err := os.RemoveAll(s.dir); err != nil {
		return err
	}
	return e
}
