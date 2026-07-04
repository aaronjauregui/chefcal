// Package caldav provides a minimal CalDAV client for publishing calendar
// resources to a Nextcloud calendar collection. It speaks just enough of the
// protocol for ChefCal's push model: ensure the collection exists (MKCALENDAR),
// list its resources (PROPFIND), and create/update/remove individual event
// resources (PUT/DELETE).
package caldav

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Client talks to a single CalDAV calendar collection.
type Client struct {
	baseURL  string // collection URL, without trailing slash
	username string
	password string
	http     *http.Client
}

// NewClient returns a client bound to the given calendar collection URL
// (e.g. https://host/remote.php/dav/calendars/user/chefcal).
func NewClient(calendarURL, username, password string, insecureSkipVerify bool) *Client {
	hc := &http.Client{Timeout: 30 * time.Second}
	if insecureSkipVerify {
		hc.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return &Client{
		baseURL:  strings.TrimRight(calendarURL, "/"),
		username: username,
		password: password,
		http:     hc,
	}
}

func (c *Client) do(method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.http.Do(req)
}

// EnsureCalendar creates the calendar collection with the given display name.
// It is idempotent: an already-existing collection is treated as success.
func (c *Client) EnsureCalendar(displayName string) error {
	body := []byte(fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<C:mkcalendar xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:caldav">
  <D:set><D:prop><D:displayname>%s</D:displayname></D:prop></D:set>
</C:mkcalendar>`, xmlEscape(displayName)))

	resp, err := c.do("MKCALENDAR", c.baseURL, body, map[string]string{
		"Content-Type": "application/xml; charset=utf-8",
	})
	if err != nil {
		return fmt.Errorf("mkcalendar: %w", err)
	}
	defer drain(resp)

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusMethodNotAllowed, http.StatusForbidden:
		// Collection already exists (Nextcloud returns 405, some servers 403).
		return nil
	default:
		return fmt.Errorf("mkcalendar: unexpected status %s", resp.Status)
	}
}

// List returns the resource basenames present in the collection.
func (c *Client) List() ([]string, error) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>`)

	resp, err := c.do("PROPFIND", c.baseURL+"/", body, map[string]string{
		"Content-Type": "application/xml; charset=utf-8",
		"Depth":        "1",
	})
	if err != nil {
		return nil, fmt.Errorf("propfind: %w", err)
	}
	defer drain(resp)

	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("propfind: unexpected status %s", resp.Status)
	}

	var ms multistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, fmt.Errorf("propfind: parsing response: %w", err)
	}

	var names []string
	for _, r := range ms.Responses {
		href := strings.TrimRight(r.Href, "/")
		name := path.Base(href)
		if strings.HasSuffix(name, ".ics") {
			// href may be percent-encoded
			if decoded, err := url.PathUnescape(name); err == nil {
				name = decoded
			}
			names = append(names, name)
		}
	}
	return names, nil
}

// Put creates or replaces the resource with the given basename.
func (c *Client) Put(name, ics string) error {
	resp, err := c.do("PUT", c.resourceURL(name), []byte(ics), map[string]string{
		"Content-Type": "text/calendar; charset=utf-8",
	})
	if err != nil {
		return fmt.Errorf("put %s: %w", name, err)
	}
	defer drain(resp)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("put %s: unexpected status %s", name, resp.Status)
	}
	return nil
}

// Delete removes the resource with the given basename. A missing resource is
// treated as success.
func (c *Client) Delete(name string) error {
	resp, err := c.do("DELETE", c.resourceURL(name), nil, nil)
	if err != nil {
		return fmt.Errorf("delete %s: %w", name, err)
	}
	defer drain(resp)

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete %s: unexpected status %s", name, resp.Status)
	}
	return nil
}

func (c *Client) resourceURL(name string) string {
	return c.baseURL + "/" + url.PathEscape(name)
}

type multistatus struct {
	XMLName   xml.Name `xml:"DAV: multistatus"`
	Responses []struct {
		Href string `xml:"DAV: href"`
	} `xml:"DAV: response"`
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
