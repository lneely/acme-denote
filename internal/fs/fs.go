package fs

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// Metadata is the metadata encoded into Denote-style
// file names.
type Metadata struct {
	Path       string
	Identifier string
	Title      string
	Tags       []string
	Extension  string
}
type Results []*Metadata

func (es Results) Bytes() []byte {
	var buf strings.Builder
	for _, e := range es {
		title := e.Title
		if title == "" {
			title = "(untitled)"
		}

		tags := strings.Join(e.Tags, ", ")
		fmt.Fprintf(&buf, "denote:%s | %s | %s\n", e.Identifier, title, tags)
	}
	return []byte(buf.String())
}

// Filter matches a given field in a Result to a regular expression
type Filter struct {
	field  FilterField
	re     *regexp.Regexp
	negate bool
}

type Filters []*Filter

type FilterField string

const (
	FilterDate  FilterField = "date"
	FilterTitle FilterField = "title"
	FilterTag   FilterField = "tag"
	FilterAny   FilterField = ""
)

// Parse converts a slice of strings of the form "tag:<tagname>",
// "date:<date>", "title:'<title>'" into a Filters list
func (fs Filters) Parse(S []string) (Filters, error) {
	for _, fa := range S {
		f, err := NewFilter(fa)
		if err != nil {
			return nil, fmt.Errorf("failed to list notes: %w", err)
		}
		fs = append(fs, f)
	}
	return fs, nil
}

// NewFilter constructs a Filter from a filter string. arg takes the form
// field:criteria, e.g., tag:/dev|meeting/, date:20251101.
func NewFilter(arg string) (*Filter, error) {
	negate := strings.HasPrefix(arg, "!")
	if negate {
		arg = strings.TrimPrefix(arg, "!")
	}

	m := regexp.MustCompile(`^(?:(date|title|tag):)?(.+)$`).FindStringSubmatch(arg)
	if m == nil {
		return nil, fmt.Errorf("invalid filter syntax: %s", arg)
	}

	pattern := m[2]
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimPrefix(strings.TrimSuffix(pattern, "/"), "/")
	} else {
		pattern = regexp.QuoteMeta(pattern)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %v", err)
	}

	return &Filter{field: FilterField(m[1]), re: re, negate: negate}, nil
}

// IsMatch checks if a note matches this filter
func (f *Filter) IsMatch(n *Metadata) bool {
	result := false
	switch f.field {
	case FilterDate:
		result = f.re.MatchString(n.Identifier)
	case FilterTitle:
		result = f.re.MatchString(n.Title)
	case FilterTag:
		result = slices.ContainsFunc(n.Tags, func(kw string) bool {
			return f.re.MatchString(kw)
		})
	case FilterAny: // any field
		if f.re.MatchString(n.Identifier) {
			result = true
		} else if f.re.MatchString(n.Title) {
			result = true
		} else {
			result = slices.ContainsFunc(n.Tags, func(kw string) bool {
				return f.re.MatchString(kw)
			})
		}
	default:
		return false
	}
	if f.negate {
		return !result
	}
	return result
}

type SortBy string

const (
	SortById    SortBy = "id"
	SortByDate  SortBy = "date"
	SortByTitle SortBy = "title"
)

type SortOrder int

const (
	SortOrderAsc SortOrder = iota
	SortOrderDesc
)

// Sort organizes a list of notes by sortType and order using metadata.
func Sort(md Results, sortType SortBy, order SortOrder) {
	switch sortType {
	case SortById, SortByDate:
		sort.Slice(md, func(i, j int) bool {
			if order == SortOrderAsc {
				return md[i].Identifier < md[j].Identifier // Reverse chronological by default
			} else {
				return md[i].Identifier > md[j].Identifier // Reverse chronological by default
			}
		})
	case SortByTitle:
		sort.Slice(md, func(i, j int) bool {
			if order == SortOrderAsc {
				return strings.ToLower(md[i].Title) < strings.ToLower(md[j].Title)
			} else {
				return strings.ToLower(md[i].Title) > strings.ToLower(md[j].Title)
			}
		})
	default:
		sort.Slice(md, func(i, j int) bool {
			return md[i].Identifier > md[j].Identifier // Reverse chronological by default
		})
	}
}

// File types in our filesystem
const (
	QTDir  = plan9.QTDIR
	QTFile = plan9.QTFILE
)

// Qid paths - we'll use a simple scheme:
// 0: root
// 1-1000000: note directories (qid = 1 + note_index)
// 1000001+: files (qid = 1000001 + note_index*100 + file_type)
const (
	qidRoot     = 0
	qidNoteBase = 1
	qidFileBase = 1000001
	qidIndex    = 999999
	qidEvent    = 999998
)

// File types within a note directory
const (
	fileTypePath = iota
	fileTypeTitle
	fileTypeKeywords
	fileTypeExtension
)

var fileNames = []string{"path", "title", "keywords", "extension"}

type server struct {
	dir          string
	notes        Results
	mu           sync.RWMutex
	eventChan    chan string
	eventSubs    map[uint64]chan string
	eventMu      sync.RWMutex
	nextSubID    uint64
	nextSubIDMu  sync.Mutex
}

type connState struct {
	fids map[uint32]*fid
	mu   sync.RWMutex
}

type fid struct {
	qid    plan9.Qid
	path   string
	offset int64
	mode   uint8
	subID  uint64 // unique event subscriber ID, 0 if not subscribed
}

var srv *server

// extractMetadata extracts Denote metadata from a filename
func extractMetadata(path string) *Metadata {
	fname := filepath.Base(path)
	note := &Metadata{Path: path}

	if m := regexp.MustCompile(`^(\d{8}T\d{6})`).FindStringSubmatch(fname); m != nil {
		note.Identifier = m[1]
	}

	// Extract title from filename
	filenameTitle := ""
	if m := regexp.MustCompile(`--([^_\.]+)`).FindStringSubmatch(fname); m != nil {
		filenameTitle = strings.ReplaceAll(m[1], "-", " ")
	}

	// Try to get title from file content, fall back to filename
	fileContentTitle := extractTitle(path)
	if fileContentTitle != "" {
		note.Title = fileContentTitle
	} else {
		note.Title = filenameTitle
	}

	if m := regexp.MustCompile(`__(.+?)(?:\.|$)`).FindStringSubmatch(fname); m != nil {
		note.Tags = strings.Split(m[1], "_")
	}
	note.Extension = filepath.Ext(fname)

	return note
}

func extractTitle(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".org" && ext != ".md" && ext != ".txt" {
		return ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(data)

	// Try org-mode #+title: first, then fall back to first heading
	if ext == ".org" {
		if m := regexp.MustCompile(`(?m)^#\+title:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
		// Fallback to first heading (lines starting with *)
		if m := regexp.MustCompile(`(?m)^\*+\s+(.+)$`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
	}

	// Try markdown YAML front matter title: first, then fall back to # header
	if ext == ".md" {
		if m := regexp.MustCompile(`(?ms)^---\n.*?^title:\s*(.+?)$.*?^---`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(strings.Trim(m[1], `"`))
		}
		if m := regexp.MustCompile(`(?m)^#\s+(.+)$`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
	}

	return ""
}

// Getdir returns the denote directory
func getdir() string {
	return fmt.Sprintf("%s/doc", os.Getenv("HOME"))
}

// loadData returns metadata for notes in a directory matching given filters
func loadData(filters []*Filter) (Results, error) {
	var notes Results
	err := filepath.Walk(getdir(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if note := extractMetadata(path); note.Identifier != "" {
			match := true
			for _, filt := range filters {
				if !filt.IsMatch(note) {
					match = false
					break
				}
			}
			if match {
				notes = append(notes, note)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return notes, nil
}

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

// StartServer starts the 9P fileserver in the background
func StartServer() error {
	if srv != nil {
		return fmt.Errorf("server already running")
	}

	// Load all notes
	notes, err := loadData([]*Filter{})
	if err != nil {
		return fmt.Errorf("failed to load notes: %w", err)
	}

	srv = &server{
		dir:       getdir(),
		notes:     notes,
		eventChan: make(chan string, 100),
		eventSubs: make(map[uint64]chan string),
	}

	// Start broadcaster goroutine
	go srv.broadcastEvents()

	// Get namespace and create Unix socket path
	ns := client.Namespace()
	if ns == "" {
		return fmt.Errorf("failed to get namespace")
	}

	sockPath := ns + "/denote"

	// Remove old socket if it exists
	os.Remove(sockPath)

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}

	// Start accepting connections in background
	go srv.acceptLoop(listener)

	return nil
}

func (s *server) broadcastEvents() {
	for event := range s.eventChan {
		s.eventMu.RLock()
		for _, sub := range s.eventSubs {
			select {
			case sub <- event:
			default:
				// Subscriber slow, drop event
			}
		}
		s.eventMu.RUnlock()
	}
}

func (s *server) acceptLoop(listener net.Listener) {
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "denote fs: accept error: %v\n", err)
			return
		}

		go s.serve(conn)
	}
}

func (s *server) serve(conn net.Conn) {
	defer conn.Close()

	cs := &connState{
		fids: make(map[uint32]*fid),
	}

	for {
		fc, err := plan9.ReadFcall(conn)
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "denote fs: read error: %v\n", err)
			}
			return
		}

		rfc := s.handle(cs, fc)
		if err := plan9.WriteFcall(conn, rfc); err != nil {
			fmt.Fprintf(os.Stderr, "denote fs: write error: %v\n", err)
			return
		}
	}
}

func (s *server) handle(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	switch fc.Type {
	case plan9.Tversion:
		return s.version(fc)
	case plan9.Tauth:
		return errorFcall(fc, "denote: authentication not required")
	case plan9.Tattach:
		return s.attach(cs, fc)
	case plan9.Twalk:
		return s.walk(cs, fc)
	case plan9.Topen:
		return s.open(cs, fc)
	case plan9.Tread:
		return s.read(cs, fc)
	case plan9.Twrite:
		return s.write(cs, fc)
	case plan9.Tstat:
		return s.stat(cs, fc)
	case plan9.Tclunk:
		return s.clunk(cs, fc)
	default:
		return errorFcall(fc, "operation not supported")
	}
}

func (s *server) version(fc *plan9.Fcall) *plan9.Fcall {
	msize := min(fc.Msize, 8192)
	return &plan9.Fcall{
		Type:    plan9.Rversion,
		Tag:     fc.Tag,
		Msize:   msize,
		Version: "9P2000",
	}
}

func (s *server) attach(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	qid := plan9.Qid{
		Type: QTDir,
		Path: qidRoot,
	}

	cs.fids[fc.Fid] = &fid{
		qid:  qid,
		path: "/",
	}

	return &plan9.Fcall{
		Type: plan9.Rattach,
		Tag:  fc.Tag,
		Qid:  qid,
	}
}

func (s *server) walk(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	f, ok := cs.fids[fc.Fid]
	if !ok {
		return errorFcall(fc, "fid not found")
	}

	// If no wnames, this is a clone operation
	if len(fc.Wname) == 0 {
		newFid := &fid{
			qid:  f.qid,
			path: f.path,
		}
		cs.fids[fc.Newfid] = newFid
		return &plan9.Fcall{
			Type: plan9.Rwalk,
			Tag:  fc.Tag,
			Wqid: []plan9.Qid{},
		}
	}

	// Walk the path
	path := f.path
	qids := []plan9.Qid{}

	for _, name := range fc.Wname {
		if path == "/" {
			// Walking from root - should be a note ID
			found := false
			if "index" == name {
				qid := plan9.Qid{
					Type: QTFile,
					Path: uint64(qidIndex),
				}
				qids = append(qids, qid)
				path = "/index"
				found = true
			} else if "event" == name {
				qid := plan9.Qid{
					Type: QTFile,
					Path: uint64(qidEvent),
				}
				qids = append(qids, qid)
				path = "/event"
				found = true
			} else {
				for i, note := range s.notes {
					if note.Identifier == name {
						qid := plan9.Qid{
							Type: QTDir,
							Path: uint64(qidNoteBase + i),
						}
						qids = append(qids, qid)
						path = "/" + name
						found = true
						break
					}
				}
			}
			if !found {
				return errorFcall(fc, "file not found")
			}
		} else {
			// Walking from note dir - should be a file
			found := false
			for i, fname := range fileNames {
				if fname == name {
					noteIdx := s.pathToNoteIndex(path)
					qid := plan9.Qid{
						Type: QTFile,
						Path: uint64(qidFileBase + noteIdx*100 + i),
					}
					qids = append(qids, qid)
					path = path + "/" + name
					found = true
					break
				}
			}
			if !found {
				return errorFcall(fc, "file not found")
			}
		}
	}

	newFid := &fid{
		qid:  qids[len(qids)-1],
		path: path,
	}
	cs.fids[fc.Newfid] = newFid

	return &plan9.Fcall{
		Type: plan9.Rwalk,
		Tag:  fc.Tag,
		Wqid: qids,
	}
}

func (s *server) open(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	f, ok := cs.fids[fc.Fid]
	if !ok {
		return errorFcall(fc, "fid not found")
	}

	f.mode = fc.Mode

	// Register event subscriber if opening event file
	if f.qid.Path == uint64(qidEvent) {
		s.nextSubIDMu.Lock()
		s.nextSubID++
		subID := s.nextSubID
		s.nextSubIDMu.Unlock()

		f.subID = subID

		s.eventMu.Lock()
		s.eventSubs[subID] = make(chan string, 10)
		s.eventMu.Unlock()
	}

	return &plan9.Fcall{
		Type: plan9.Ropen,
		Tag:  fc.Tag,
		Qid:  f.qid,
	}
}

func (s *server) read(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()

	f, ok := cs.fids[fc.Fid]
	if !ok {
		cs.mu.Unlock()
		return errorFcall(fc, "fid not found")
	}

	// Handle event file reads specially
	if f.qid.Path == uint64(qidEvent) {
		subID := f.subID
		s.eventMu.RLock()
		eventChan, ok := s.eventSubs[subID]
		s.eventMu.RUnlock()
		cs.mu.Unlock()

		if !ok {
			return errorFcall(fc, "event subscriber not found")
		}

		// Block waiting for next event (without holding server mutex)
		event, ok := <-eventChan
		if !ok {
			return errorFcall(fc, "event channel closed")
		}
		data := []byte(event + "\n")

		return &plan9.Fcall{
			Type:  plan9.Rread,
			Tag:   fc.Tag,
			Count: uint32(len(data)),
			Data:  data,
		}
	}

	defer cs.mu.Unlock()

	var data []byte

	if f.qid.Type&QTDir != 0 {
		// Reading a directory
		if fc.Offset == 0 {
			f.offset = 0
		}
		data = s.readDir(f.path, int64(fc.Offset), fc.Count)
	} else {
		// Reading a file
		content := s.getFileContent(f.path)
		offset := int64(fc.Offset)
		if offset >= int64(len(content)) {
			data = []byte{}
		} else {
			end := min(offset+int64(fc.Count), int64(len(content)))
			data = []byte(content[offset:end])
		}
	}

	return &plan9.Fcall{
		Type:  plan9.Rread,
		Tag:   fc.Tag,
		Count: uint32(len(data)),
		Data:  data,
	}
}

func (s *server) write(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	f, ok := cs.fids[fc.Fid]
	if !ok {
		return errorFcall(fc, "fid not found")
	}

	// Check if opened for writing
	if f.mode&plan9.OWRITE == 0 && f.mode&plan9.ORDWR == 0 {
		return errorFcall(fc, "file not open for writing")
	}

	// Extract note identifier and field name from path
	parts := strings.Split(strings.Trim(f.path, "/"), "/")
	if len(parts) != 2 {
		return errorFcall(fc, "invalid path")
	}

	noteID := parts[0]
	fieldName := parts[1]

	// Find note in s.notes
	var note *Metadata
	for _, n := range s.notes {
		if n.Identifier == noteID {
			note = n
			break
		}
	}

	if note == nil {
		return errorFcall(fc, "note not found")
	}

	// Update the field in memory only
	value := strings.TrimSpace(string(fc.Data))

	switch fieldName {
	case "title":
		note.Title = value
	case "keywords":
		if value == "" {
			note.Tags = []string{}
		} else {
			tags := strings.Split(value, ",")
			for i := range tags {
				tags[i] = strings.TrimSpace(tags[i])
			}
			note.Tags = tags
		}
	default:
		return errorFcall(fc, "field is read-only")
	}

	// Emit event for metadata update
	event := noteID + " u"
	select {
	case s.eventChan <- event:
	default:
		// Channel full, drop event
	}

	return &plan9.Fcall{
		Type:  plan9.Rwrite,
		Tag:   fc.Tag,
		Count: uint32(len(fc.Data)),
	}
}

func (s *server) stat(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	f, ok := cs.fids[fc.Fid]
	if !ok {
		return errorFcall(fc, "fid not found")
	}

	dir := s.pathToDir(f.path, f.qid)
	stat, err := dir.Bytes()
	if err != nil {
		return errorFcall(fc, err.Error())
	}

	return &plan9.Fcall{
		Type: plan9.Rstat,
		Tag:  fc.Tag,
		Stat: stat,
	}
}

func (s *server) clunk(cs *connState, fc *plan9.Fcall) *plan9.Fcall {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Unregister event subscriber if this fid has one
	if f, ok := cs.fids[fc.Fid]; ok && f.subID != 0 {
		s.eventMu.Lock()
		if ch, ok := s.eventSubs[f.subID]; ok {
			close(ch)
			delete(s.eventSubs, f.subID)
		}
		s.eventMu.Unlock()
	}

	delete(cs.fids, fc.Fid)

	return &plan9.Fcall{
		Type: plan9.Rclunk,
		Tag:  fc.Tag,
	}
}

func (s *server) readDir(path string, offset int64, count uint32) []byte {
	var dirs []plan9.Dir

	if path == "/" {
		// add index node
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{
				Type: QTFile,
				Path: uint64(qidIndex),
			},
			Mode:   0444,
			Name:   "index",
			Uid:    "denote",
			Gid:    "denote",
			Muid:   "denote",
			Length: 0,
		})
		// add event node
		dirs = append(dirs, plan9.Dir{
			Qid: plan9.Qid{
				Type: QTFile,
				Path: uint64(qidEvent),
			},
			Mode:   0444,
			Name:   "event",
			Uid:    "denote",
			Gid:    "denote",
			Muid:   "denote",
			Length: 0,
		})
		// List all note IDs
		for i, note := range s.notes {
			dirs = append(dirs, plan9.Dir{
				Qid: plan9.Qid{
					Type: QTDir,
					Path: uint64(qidNoteBase + 1 + i),
				},
				Mode:   plan9.DMDIR | 0555,
				Name:   note.Identifier,
				Uid:    "denote",
				Gid:    "denote",
				Muid:   "denote",
				Length: 0,
			})
		}
	} else if path == "/index" {

	} else {
		// List files in a note directory
		noteIdx := s.pathToNoteIndex(path)
		for i, fname := range fileNames {
			content := s.getFileContent(path + "/" + fname)
			mode := uint32(0444) // read-only by default
			if fname == "title" || fname == "keywords" {
				mode = 0644 // writable
			}
			dirs = append(dirs, plan9.Dir{
				Qid: plan9.Qid{
					Type: QTFile,
					Path: uint64(qidFileBase + noteIdx*100 + i),
				},
				Mode:   plan9.Perm(mode),
				Name:   fname,
				Uid:    "denote",
				Gid:    "denote",
				Muid:   "denote",
				Length: uint64(len(content)),
			})
		}
	}

	// Serialize all directory entries to bytes first
	var allData []byte
	for _, dir := range dirs {
		stat, _ := dir.Bytes()
		allData = append(allData, stat...)
	}

	// Return slice starting from offset
	if offset >= int64(len(allData)) {
		return []byte{}
	}

	end := offset + int64(count)
	if end > int64(len(allData)) {
		end = int64(len(allData))
	}

	return allData[offset:end]
}

func (s *server) pathToDir(path string, qid plan9.Qid) plan9.Dir {
	name := path
	if path == "/" {
		name = "."
	} else if strings.Contains(path, "/") {
		parts := strings.Split(path, "/")
		name = parts[len(parts)-1]
	}

	mode := uint32(0444)
	length := uint64(0)
	content := ""

	if qid.Type&QTDir != 0 {
		mode = plan9.DMDIR | 0555
	} else {
		content = s.getFileContent(path)
		length = uint64(len(content))
	}

	return plan9.Dir{
		Qid:    qid,
		Mode:   plan9.Perm(mode),
		Name:   name,
		Uid:    "denote",
		Gid:    "denote",
		Muid:   "denote",
		Length: length,
	}
}

func (s *server) pathToNoteIndex(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return -1
	}

	noteID := parts[0]
	for i, note := range s.notes {
		if note.Identifier == noteID {
			return i
		}
	}
	return -1
}

func (s *server) getIndexContent() string {
	content := ""
	for _, n := range s.notes {
		content += fmt.Sprintf("%s|%s|", n.Identifier, n.Title)
		for _, t := range n.Tags {
			content += fmt.Sprintf("%s,", t)
		}
		content = strings.TrimSuffix(content, ",")
		content += "\n"
	}
	return content
}

func (s *server) getFileContent(path string) string {
	if path == "/index" {
		return s.getIndexContent()
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		return ""
	}

	noteID := parts[0]
	fileName := parts[1]

	var note *Metadata
	for _, n := range s.notes {
		if n.Identifier == noteID {
			note = n
			break
		}
	}

	if note == nil {
		return ""
	}

	switch fileName {
	case "path":
		return note.Path
	case "title":
		return note.Title
	case "keywords":
		return strings.Join(note.Tags, ",")
	case "extension":
		return note.Extension
	}
	return ""
}

func errorFcall(fc *plan9.Fcall, msg string) *plan9.Fcall {
	return &plan9.Fcall{
		Type:  plan9.Rerror,
		Tag:   fc.Tag,
		Ename: msg,
	}
}
