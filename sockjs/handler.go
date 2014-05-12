package sockjs

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

type handler struct {
	prefix      string
	options     Options
	handlerFunc HandlerFunc
	mappings    []*mapping

	sessionsMux sync.Mutex
	sessions    map[string]*session
}

// NewHandler creates new HTTP handler that conforms to the basic net/http.Handler interface.
// It takes path prefix, options and sockjs handler function as parameters
func NewHandler(prefix string, opts Options, handlerFn HandlerFunc) *handler {
	h := &handler{
		prefix:      prefix,
		options:     opts,
		handlerFunc: handlerFn,
		sessions:    make(map[string]*session),
	}

	sessionPrefix := prefix + "/[^/.]+/[^/.]+"
	h.mappings = []*mapping{
		newMapping("GET", prefix+"[/]?$", welcomeHandler),
		newMapping("OPTIONS", prefix+"/info?$", opts.cookie, xhrCors, cacheFor, opts.info),
		newMapping("GET", prefix+"/info?$", xhrCors, noCache, opts.info),
		//
		// other mappings
		newMapping("POST", sessionPrefix+"/xhr_send$", opts.cookie, xhrCors, noCache, h.xhrSend),
		newMapping("OPTIONS", sessionPrefix+"/xhr_send$", opts.cookie, xhrCors, cacheFor, xhrOptions),
		newMapping("POST", sessionPrefix+"/xhr$", opts.cookie, xhrCors, noCache, h.xhrPoll),
		newMapping("OPTIONS", sessionPrefix+"/xhr$", opts.cookie, xhrCors, cacheFor, xhrOptions),
		newMapping("POST", sessionPrefix+"/xhr_streaming$", opts.cookie, xhrCors, noCache, h.xhrStreaming),
		newMapping("OPTIONS", sessionPrefix+"/xhr_streaming$", opts.cookie, xhrCors, cacheFor, xhrOptions),
		newMapping("GET", sessionPrefix+"/eventsource$", opts.cookie, xhrCors, noCache, h.eventSource),
		newMapping("GET", sessionPrefix+"/htmlfile$", opts.cookie, xhrCors, noCache, h.htmlFile),
		newMapping("GET", sessionPrefix+"/jsonp$", opts.cookie, xhrCors, noCache, h.jsonp),
		newMapping("OPTIONS", sessionPrefix+"/jsonp$", opts.cookie, xhrCors, cacheFor, xhrOptions),
		newMapping("POST", sessionPrefix+"/jsonp_send$", opts.cookie, xhrCors, noCache, h.jsonp_send),
		newMapping("GET", prefix+"/iframe[0-9-.a-z_]*.html$", cacheFor, h.iframe),
	}
	return h
}

func (h *handler) Prefix() string { return h.prefix }

func (h *handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// iterate over mappings
	allowedMethods := []string{}
	for _, mapping := range h.mappings {
		if match, method := mapping.matches(req); match == fullMatch {
			for _, hf := range mapping.chain {
				hf(rw, req)
			}
			return
		} else if match == pathMatch {
			allowedMethods = append(allowedMethods, method)
		}
	}
	if len(allowedMethods) > 0 {
		rw.Header().Set("allow", strings.Join(allowedMethods, ", "))
		rw.Header().Set("Content-Type", "")
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	http.NotFound(rw, req)
}

func (h *handler) parseSessionID(url *url.URL) (string, error) {
	session := regexp.MustCompile(h.prefix + "/(?P<server>[^/.]+)/(?P<session>[^/.]+)/.*")
	matches := session.FindStringSubmatch(url.Path)
	if len(matches) == 3 {
		return matches[2], nil
	}
	return "", errors.New("unable to parse URL for session")
}

func (h *handler) sessionByRequest(req *http.Request) (*session, error) {
	h.sessionsMux.Lock()
	defer h.sessionsMux.Unlock()
	sessionID, err := h.parseSessionID(req.URL)
	if err != nil {
		return nil, err
	}
	sess, exists := h.sessions[sessionID]
	if !exists {
		sess = newSession(h.options.DisconnectDelay, h.options.HeartbeatDelay)
		h.sessions[sessionID] = sess
		if h.handlerFunc != nil {
			go h.handlerFunc(sess)
		}
		go func() {
			<-sess.closedNotify()
			h.sessionsMux.Lock()
			delete(h.sessions, sessionID)
			h.sessionsMux.Unlock()
		}()
	}
	return sess, nil
}
