package sockjs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var iframe_template string = `<!doctype html>
<html><head>
  <meta http-equiv="X-UA-Compatible" content="IE=edge" />
  <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
</head><body><h2>Don't panic!</h2>
  <script>
    document.domain = document.domain;
    var c = parent.%s;
    c.start();
    function p(d) {c.message(d);};
    window.onload = function() {c.stop();};
  </script>
`

func init() {
	iframe_template += strings.Repeat(" ", 1024-len(iframe_template)+14)
	iframe_template += "\r\n\r\n"
}

func (h *handler) htmlFile(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("content-type", "text/html; charset=UTF-8")

	req.ParseForm()
	callback := req.Form.Get("c")
	if callback == "" {
		http.Error(rw, `"callback" parameter required`, http.StatusInternalServerError)
		return
	}
	rw.WriteHeader(http.StatusOK)
	rw.(http.Flusher).Flush()
	fmt.Fprintf(rw, iframe_template, callback)
	rw.(http.Flusher).Flush()

	sess, _ := h.sessionByRequest(req)
	recv := newHttpReceiver(rw, h.options.ResponseLimit, new(htmlfileFrameWriter))
	if err := sess.attachReceiver(recv); err != nil {
		recv.sendFrame(cFrame)
		return
	}
	select {
	case <-recv.doneNotify():
	case <-recv.interruptedNotify():
	}
}

type htmlfileFrameWriter struct{}

func (*htmlfileFrameWriter) write(w io.Writer, frame string) (int, error) {
	payload, _ := json.Marshal(frame)
	return fmt.Fprintf(w, "<script>\np(%s);\n</script>\r\n", string(payload))
}
