package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var bufferPool sync.Pool
var proxyAddr string = mustGetenv("PROXY_ADDR")
var dnsSuffix string = mustGetenv("CERTIFICATE_DNS_SUFFIX")

func init() {
	makeBuffer := func() interface{} { return make([]byte, 0, 32*1024) }
	bufferPool = sync.Pool{New: makeBuffer}
}

func main() {
	// Create a CA certificate pool and add cert.pem to it
	caCert, err := os.ReadFile("cacert.pem")
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create the TLS Config with the CA pool and enable Client certificate validation
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(verifiedChains) > 0 {
				if len(verifiedChains[0]) > 0 {
					if len(verifiedChains[0][0].DNSNames) > 0 {
						if strings.HasSuffix(verifiedChains[0][0].DNSNames[0], dnsSuffix) {
							return nil
						}
					}
				}
			}

			return fmt.Errorf("invalid certificate hostname")
		},
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8443"
		log.Printf("Defaulting to port %s", port)
	}

	// Create a Server instance to listen on port 8443 with the TLS config
	server := &http.Server{
		Addr:      ":" + port,
		TLSConfig: tlsConfig,
		Handler:   http.HandlerFunc(proxyHandler),
	}

	// Listen to HTTPS connections with the server certificate and wait
	log.Fatal(server.ListenAndServeTLS("cert.pem", "key.pem"))
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	// First, verify the mTLS CN / attributes
	identity, ok := strings.CutSuffix(r.TLS.VerifiedChains[0][0].DNSNames[0], dnsSuffix)
	if !ok {
		http.Error(w, "Permission Denied", http.StatusForbidden)
		return
	}

	log.Printf("%s %s %s %s", r.RemoteAddr, identity, r.Method, r.URL)

	if r.Method == http.MethodConnect {
		if r.ProtoMajor == 2 {
			if len(r.URL.Scheme) > 0 || len(r.URL.Path) > 0 {
				log.Printf("CONNECT request has :scheme or/and :path pseudo-header fields")
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
		}
	}

	// Add a "Proxy-Authorization" header, to notify squid who we've got.
	r.Header.Set("Proxy-Authorization", "Basic "+basicAuth(identity, "automatic"))
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	r.Header.Set("X-Forwarded-For", host)
	r.Header.Set("X-Real-IP", host)

	// Pass the connection to squid...
	targetConn, err := net.DialTimeout("tcp", proxyAddr, time.Second*2)
	if err != nil {
		log.Printf("failed to connect to upstream proxy: %s", err)
		return
	}
	defer targetConn.Close()

	req := r
	if r.Method == "CONNECT" {
		req = &http.Request{
			Method: "CONNECT",
			URL:    r.URL,
			Host:   r.Host,
			Header: r.Header.Clone(),
		}
	} else {
		r.URL.Scheme = "http"
	}
	err = req.WriteProxy(targetConn)
	if err != nil {
		log.Printf("failed to write request to upstream proxy: %s", err)
		return
	}

	switch r.ProtoMajor {
	case 1: // http1: hijack the whole flow
		err = serveHijack(w, targetConn)
		if err != nil {
			log.Printf("%s", err)
		}
	case 2: // http2: keep reading from "request" and writing into same response
		defer r.Body.Close()

		wFlusher, ok := w.(http.Flusher)
		if !ok {
			log.Printf("ResponseWriter doesn't implement Flusher()")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Okay to use and discard buffered reader here, because
		// TLS server will not speak until spoken to.
		br := bufio.NewReader(targetConn)
		resp, err := http.ReadResponse(br, r)
		if err != nil {
			log.Printf("failed to read CONNECT response: %s", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		for header, values := range resp.Header {
			for _, val := range values {
				w.Header().Add(header, val)
			}
		}
		w.WriteHeader(resp.StatusCode)
		wFlusher.Flush()

		if n := br.Buffered(); n > 0 {
			rbuf, err := br.Peek(n)
			if err != nil {
				log.Printf("failed to write buffer: %s", err)
				return
			}
			w.Write(rbuf)
			wFlusher.Flush()
		}

		err = dualStream(targetConn, r.Body, w)
		if err != nil {
			log.Printf("%s", err)
		}
	default:
		panic("There was a check for http version, yet it's incorrect")
	}
}

// Provies an encoded basic authentication header value
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// Hijacks the connection from ResponseWriter, writes the response and proxies data between targetConn
// and hijacked connection.
func serveHijack(w http.ResponseWriter, targetConn net.Conn) error {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return fmt.Errorf("ResponseWriter does not implement Hijacker")
	}

	clientConn, bufReader, err := hijacker.Hijack()
	if err != nil {
		return fmt.Errorf("failed to hijack: %s", err)
	}
	defer clientConn.Close()

	// bufReader may contain unprocessed buffered data from the client.
	if bufReader != nil {
		// snippet borrowed from `proxy` plugin
		if n := bufReader.Reader.Buffered(); n > 0 {
			rbuf, err := bufReader.Reader.Peek(n)
			if err != nil {
				return err
			}
			targetConn.Write(rbuf)
		}
	}

	return dualStream(targetConn, clientConn, clientConn)
}

// Copies data target->clientReader and clientWriter->target, and flushes as needed
// Returns when clientWriter-> target stream is done.
// Caddy should finish writing target -> clientReader.
func dualStream(target net.Conn, clientReader io.ReadCloser, clientWriter io.Writer) error {
	stream := func(w io.Writer, r io.Reader) error {
		// copy bytes from r to w
		buf := bufferPool.Get().([]byte)
		buf = buf[0:cap(buf)]
		_, _err := flushingIoCopy(w, r, buf)
		if closeWriter, ok := w.(interface {
			CloseWrite() error
		}); ok {
			closeWriter.CloseWrite()
		}
		return _err
	}

	go stream(target, clientReader)
	return stream(clientWriter, target)
}

// flushingIoCopy is analogous to buffering io.Copy(), but also attempts to flush on each iteration.
// If dst does not implement http.Flusher(e.g. net.TCPConn), it will do a simple io.CopyBuffer().
// Reasoning: http2ResponseWriter will not flush on its own, so we have to do it manually.
func flushingIoCopy(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	flusher, ok := dst.(http.Flusher)
	if !ok {
		return io.CopyBuffer(dst, src, buf)
	}
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			flusher.Flush()
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return
}

func mustGetenv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Panicf("%s environment variable not set.", k)
	}
	return v
}
