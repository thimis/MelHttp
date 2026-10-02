package server

import (
	"bytes"
	"compress/gzip"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"

	"github.com/thimis/MelHttp/internal/obfs"
)

// variants holds differently-seeded transport encodings of one response, so
// the same page looks different across requests without recompiling it
// every time. Each slot is generated on first use.
type variants struct {
	mu    sync.Mutex
	slots []*encoded
}

type encoded struct {
	plain, gz []byte
}

const maxObfEntries = 4096

// encodedVariant returns a randomly chosen encoding of res.
func (s *Server) encodedVariant(url string, res *result) (*encoded, error) {
	if !res.deterministic {
		return encode(res.body, rand.Uint64()|1) // dynamic: a fresh encoding each time
	}
	key := url + "\x00" + res.etag
	s.obfMu.Lock()
	v, ok := s.obf[key]
	if !ok {
		if len(s.obf) >= maxObfEntries {
			clear(s.obf)
		}
		v = &variants{slots: make([]*encoded, s.cfg.ObfuscateVariants)}
		s.obf[key] = v
	}
	s.obfMu.Unlock()
	i := rand.IntN(len(v.slots))
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.slots[i] == nil {
		enc, err := encode(res.body, rand.Uint64()|1)
		if err != nil {
			return nil, err
		}
		v.slots[i] = enc
	}
	return v.slots[i], nil
}

func encode(body []byte, seed uint64) (*encoded, error) {
	plain, err := obfs.Encode(body, seed)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&b, gzip.BestSpeed)
	zw.Write(plain)
	zw.Close()
	return &encoded{plain: plain, gz: b.Bytes()}, nil
}

// wantsProgram reports whether the client asked for the Malbolge transport.
func (s *Server) wantsProgram(r *http.Request) bool {
	return s.cfg.Obfuscate && r.Header.Get(obfs.AcceptHeader) == obfs.Program &&
		(r.Method == http.MethodGet || r.Method == http.MethodHead)
}

// writeProgram sends a 200 response body as Malbolge programs.
func (s *Server) writeProgram(w http.ResponseWriter, r *http.Request, url string, res *result) {
	enc, err := s.encodedVariant(url, res)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set(obfs.ContentTypeHeader, res.header.Get("Content-Type"))
	s.metrics.encoded.Add(1)
	h.Set(obfs.EncodingHeader, obfs.Program)
	h.Set("Content-Type", obfs.MediaType)
	h.Set("Cache-Control", "no-store")
	h.Del("ETag")
	h.Add("Vary", "Accept-Encoding")
	body := enc.plain
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		body = enc.gz
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		w.Write(body)
	}
}
