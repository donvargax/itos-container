package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"
)

const credentialRunBodyLimit = 4 << 20

type credentialRunExchange struct {
	Method, Path, Query             string
	RequestHeaders, ResponseHeaders http.Header
	RequestBody, ResponseBody       []byte
	Status                          int
	Truncated                       bool
}

type credentialRunJournal struct {
	mu        sync.Mutex
	exchanges []*credentialJournalEntry
	server    *http.Server
	transport *http.Transport
}

type credentialJournalEntry struct {
	metadata          credentialRunExchange
	request, response credentialJournalBody
}

type credentialJournalBody struct {
	io.ReadCloser
	mu        sync.Mutex
	data      bytes.Buffer
	truncated bool
	expected  int64
}

func (b *credentialJournalBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := min(n, credentialRunBodyLimit-b.data.Len())
	_, _ = b.data.Write(p[:kept])
	b.truncated = b.truncated || kept < n
	return n, err
}

func (b *credentialJournalBody) snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.data.Bytes()), b.truncated || b.expected > int64(b.data.Len())
}

type credentialJournalKey struct{}

func newCredentialRunJournal(socket string) (*credentialRunJournal, error) {
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	j := &credentialRunJournal{transport: doctorTransport("/var/run/docker.sock")}
	j.transport.DisableCompression = true
	proxy := &httputil.ReverseProxy{
		Transport: j.transport, FlushInterval: -1, ErrorLog: log.New(io.Discard, "", 0),
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme, r.Out.URL.Host = "http", "docker"
			// The engine interprets the original query string.
			r.Out.URL.RawQuery = r.In.URL.RawQuery
		},
		ModifyResponse: func(r *http.Response) error {
			entry := r.Request.Context().Value(credentialJournalKey{}).(*credentialJournalEntry)
			j.mu.Lock()
			defer j.mu.Unlock()
			entry.metadata.Status, entry.metadata.ResponseHeaders = r.StatusCode, r.Header.Clone()
			if r.StatusCode != http.StatusSwitchingProtocols && r.Request.Method != http.MethodHead {
				entry.response.ReadCloser = r.Body
				entry.response.expected = r.ContentLength
				r.Body = &entry.response
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			entry := r.Context().Value(credentialJournalKey{}).(*credentialJournalEntry)
			const message = "fixture engine transport failed\n"
			j.mu.Lock()
			entry.metadata.Status = http.StatusBadGateway
			entry.response.mu.Lock()
			defer entry.response.mu.Unlock()
			entry.metadata.ResponseHeaders = http.Header{"Content-Type": {"text/plain; charset=utf-8"}}
			_, _ = entry.response.data.WriteString(message)
			j.mu.Unlock()
			http.Error(w, "fixture engine transport failed", http.StatusBadGateway)
		},
	}
	j.server = &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entry := &credentialJournalEntry{metadata: credentialRunExchange{
				Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, RequestHeaders: r.Header.Clone(),
			}}
			entry.metadata.RequestHeaders.Set("Host", r.Host)
			if r.Body != nil && r.Body != http.NoBody {
				entry.request.ReadCloser = r.Body
				entry.request.expected = r.ContentLength
				r.Body = &entry.request
			}
			j.mu.Lock()
			j.exchanges = append(j.exchanges, entry)
			j.mu.Unlock()
			proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), credentialJournalKey{}, entry)))
		}),
	}
	go func() { _ = j.server.Serve(listener) }()
	return j, nil
}

func (j *credentialRunJournal) snapshot() []credentialRunExchange {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := make([]credentialRunExchange, 0, len(j.exchanges))
	for _, entry := range j.exchanges {
		exchange := entry.metadata
		request, requestTruncated := entry.request.snapshot()
		response, responseTruncated := entry.response.snapshot()
		exchange.RequestBody, exchange.ResponseBody = request, response
		exchange.Truncated = requestTruncated || responseTruncated
		result = append(result, exchange)
	}
	return result
}

func (j *credentialRunJournal) scan(scan func(string, []byte) error) error {
	for _, exchange := range j.snapshot() {
		if exchange.Truncated || exchange.Status == 0 {
			return errors.New("engine journal has an incomplete body or response")
		}
		request, response := exchange.RequestBody, exchange.ResponseBody
		exchange.RequestBody, exchange.ResponseBody = nil, nil
		header, err := json.Marshal(exchange)
		if err != nil {
			return err
		}
		for name, data := range map[string][]byte{"engine metadata": header, "engine request body": request, "engine response body": response} {
			if err := scan(name, data); err != nil {
				return err
			}
		}
	}
	return nil
}

func (j *credentialRunJournal) close() error {
	j.transport.CloseIdleConnections()
	return j.server.Close()
}
