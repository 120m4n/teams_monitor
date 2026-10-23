package actuator

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPActuator POSTs the payload as text/plain to URL (e.g. http://192.168.1.50/led).
type HTTPActuator struct {
	URL    string
	Client *http.Client
}

func NewHTTPActuator(url string) *HTTPActuator {
	return &HTTPActuator{URL: url, Client: &http.Client{Timeout: 3 * time.Second}}
}

func (a *HTTPActuator) SetCallState(active bool) error { return a.Send(payload(active)) }

func (a *HTTPActuator) Send(msg string) error {
	resp, err := a.Client.Post(a.URL, "text/plain", strings.NewReader(msg))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("actuator: %s returned %s", a.URL, resp.Status)
	}
	return nil
}
