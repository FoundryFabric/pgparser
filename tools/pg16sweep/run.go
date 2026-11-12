package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"sort"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgplex/pgparser/nodes"
	"github.com/pgplex/pgparser/parser"
)

// result is what a parser did with one corpus statement. Pos is a 0-based
// byte offset within the statement, -1 when the error has none.
type result struct {
	ID      int    `json:"id"`
	OK      bool   `json:"ok,omitempty"`
	Code    string `json:"code,omitempty"`
	Msg     string `json:"msg,omitempty"`
	Pos     int    `json:"pos"`
	Panic   string `json:"panic,omitempty"`
	Timeout bool   `json:"timeout,omitempty"`
	Fatal   bool   `json:"fatal,omitempty"`
	Micros  int64  `json:"us,omitempty"`
	// Unclassified is an oracle answer that is not a parse verdict (a
	// lost connection, a statement timeout).
	Unclassified string `json:"unclassified,omitempty"`
}

const oraclePrefix = "select 1/0;"

// byteOffset turns PostgreSQL's 1-based character position within s into a
// 0-based byte offset.
func byteOffset(s string, pos1 int) int {
	if pos1 <= 0 {
		return -1
	}
	chars := 0
	for i := range s {
		if chars == pos1-1 {
			return i
		}
		chars++
	}
	return len(s)
}

func runOracle(dsn, corpusPath, out string, workers int) error {
	items, err := readCorpus(corpusPath)
	if err != nil {
		return err
	}
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return err
	}
	cfg.RuntimeParams["statement_timeout"] = "20000"
	ctx := context.Background()
	results := make([]result, len(items))
	var next int
	var mu sync.Mutex
	var wg sync.WaitGroup
	began := time.Now()
	errc := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := pgconn.ConnectConfig(ctx, cfg)
			if err != nil {
				errc <- err
				return
			}
			defer func() { c.Close(ctx) }()
			for {
				mu.Lock()
				i := next
				next++
				mu.Unlock()
				if i >= len(items) {
					return
				}
				it := items[i]
				r := result{ID: it.ID, Pos: -1}
				t0 := time.Now()
				_, err := c.Exec(ctx, oraclePrefix+it.SQL).ReadAll()
				r.Micros = time.Since(t0).Microseconds()
				var pe *pgconn.PgError
				switch {
				case errors.As(err, &pe) && pe.Code == "22012":
					r.OK = true
				case errors.As(err, &pe) && pe.Code == "57014":
					r.Unclassified = "statement timeout"
				case errors.As(err, &pe):
					r.Code, r.Msg = pe.Code, pe.Message
					if pe.Position > 0 {
						r.Pos = byteOffset(it.SQL, int(pe.Position)-utf8.RuneCountInString(oraclePrefix))
					}
				default:
					r.Unclassified = fmt.Sprint("no error: ", err)
					if c.IsClosed() {
						c, err = pgconn.ConnectConfig(ctx, cfg)
						if err != nil {
							errc <- err
							return
						}
					}
				}
				results[i] = r
				if i%10000 == 0 {
					fmt.Fprintf(os.Stderr, "oracle: %d/%d %s\n", i, len(items), time.Since(began).Round(time.Second))
				}
			}
		}()
	}
	wg.Wait()
	close(errc)
	if err := <-errc; err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "oracle: %d statements in %s\n", len(items), time.Since(began).Round(time.Millisecond))
	return writeResults(out, results)
}

func writeResults(out string, results []result) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, r := range results {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return w.Flush()
}

func readResults(path string) (map[int]result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[int]result{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var r result
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			// A line cut short by a fatal error in the child.
			continue
		}
		m[r.ID] = r
	}
	return m, sc.Err()
}

const parseTimeout = 5 * time.Second

// parseOne parses sql, turning a panic into a result and giving up after
// parseTimeout (the goroutine is left running).
func parseOne(id int, sql string) result {
	return parseWith(id, sql, func(s string) error {
		_, err := parser.Parse(s)
		return err
	})
}

func parseFork(sql string) (*nodes.List, error) { return parser.Parse(sql) }

// parseWith is parseOne with the parse call given.
func parseWith(id int, sql string, parse func(string) error) result {
	done := make(chan result, 1)
	go func() {
		r := result{ID: id, Pos: -1}
		defer func() {
			if p := recover(); p != nil {
				r.Panic = fmt.Sprint(p) + "\n" + string(debug.Stack())
				done <- r
			}
		}()
		t0 := time.Now()
		err := parse(sql)
		r.Micros = time.Since(t0).Microseconds()
		var pe *parser.ParseError
		switch {
		case err == nil:
			r.OK = true
		case errors.As(err, &pe):
			r.Code, r.Msg, r.Pos = pe.Code, pe.Message, pe.Position
		default:
			r.Code, r.Msg = "?", err.Error()
		}
		done <- r
	}()
	select {
	case r := <-done:
		return r
	case <-time.After(parseTimeout):
		return result{ID: id, Pos: -1, Timeout: true}
	}
}

// runForkChild parses the corpus from id start on, writing each result as
// soon as it has it, so that the driver can resume after a fatal error.
func runForkChild(corpusPath, out string, start int) error {
	items, err := readCorpus(corpusPath)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, it := range items {
		if it.ID < start {
			continue
		}
		b, _ := json.Marshal(parseOne(it.ID, it.SQL))
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// runForkDriver runs the child until it has parsed every statement. When
// the child dies (a fatal error no recover catches), the statement after the
// last one it wrote is recorded as fatal and the child restarts after it.
func runForkDriver(corpusPath, out string, start int) error {
	items, err := readCorpus(corpusPath)
	if err != nil {
		return err
	}
	if start == 0 {
		_ = os.Remove(out)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	began := time.Now()
	for {
		cmd := exec.Command(exe, "fork-child", "-corpus", corpusPath, "-out", out, "-start", strconv.Itoa(start))
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		runErr := cmd.Run()
		done, err := readResults(out)
		if err != nil {
			return err
		}
		if runErr == nil {
			break
		}
		next := -1
		for _, it := range items {
			if _, ok := done[it.ID]; !ok && it.ID >= start {
				next = it.ID
				break
			}
		}
		if next < 0 {
			break
		}
		fmt.Fprintf(os.Stderr, "fork: fatal error at id %d: %v\n", next, runErr)
		b, _ := json.Marshal(result{ID: next, Pos: -1, Fatal: true})
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		_, _ = f.Write(append([]byte("\n"), append(b, '\n')...))
		f.Close()
		start = next + 1
	}
	// Rewrite in id order.
	done, err := readResults(out)
	if err != nil {
		return err
	}
	rs := make([]result, 0, len(done))
	for _, r := range done {
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
	fmt.Fprintf(os.Stderr, "fork: %d statements in %s\n", len(rs), time.Since(began).Round(time.Millisecond))
	return writeResults(out, rs)
}
