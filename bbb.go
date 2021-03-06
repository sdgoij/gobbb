package bbb

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func New(apiurl, secret string) (BigBlueButton, error) {
	b3 := BigBlueButton{Secret: secret}
	u, err := url.Parse(apiurl)
	if nil == err {
		b3.Url = u
	}
	return b3, err
}

type BigBlueButton struct {
	Secret string
	Url    *url.URL
}

func (b3 *BigBlueButton) Create(id string, options OptionEncoder) (*Meeting, error) {
	u := b3.makeURL("create", mergeUrlValues(url.Values{"meetingID": {id}}, options.Values()))
	var (
		res *http.Response
		err error
	)
	if options, ok := options.(*CreateOptions); ok && len(options.Documents) > 0 {
		if mods, oops := buildCreateMeetingXML(options.Documents); nil == oops {
			res, err = http.Post(u.String(), "text/xml", bytes.NewReader(mods))
		} else {
			err = oops
		}
	} else {
		res, err = http.Get(u.String())
	}
	if nil != err {
		return nil, err
	}
	defer res.Body.Close()
	return loadMeetingCreateResponse(res)
}

func (b3 *BigBlueButton) DefaultConfigXML() (*ConfigXML, error) {
	u := b3.makeURL("getDefaultConfigXML", url.Values{})
	res, err := http.Get(u.String())
	if nil != err {
		return nil, err
	}
	defer res.Body.Close()
	return readConfigXML(res.Body)
}

func (b3 *BigBlueButton) SetConfigXML(meeting string, c *ConfigXML) (string, error) {
	action := "setConfigXML"
	params := url.Values{
		"meetingID": {meeting},
		"configXML": {c.String()},
	}
	params.Add("checksum", b3.checksum(action, params.Encode()))
	u, err := b3.Url.Parse(action + ".xml")
	if nil != err {
		return "", err
	}
	res, err := http.PostForm(u.String(), params)
	if nil != err {
		return "", err
	}
	defer res.Body.Close()
	return loadStringResponse(res, "configToken"), nil
}

func (b3 *BigBlueButton) JoinURL(name, meetingID, password string, options OptionEncoder) string {
	return b3.makeURL("join", mergeUrlValues(
		url.Values{
			"fullName":  {name},
			"meetingID": {meetingID},
			"password":  {password},
		},
		options.Values())).String()
}

func (b3 *BigBlueButton) IsMeetingRunning(id string) bool {
	u := b3.makeURL("isMeetingRunning", url.Values{"meetingID": {id}})
	res, err := http.Get(u.String())
	if nil != err {
		return false
	}
	defer res.Body.Close()
	return loadBoolResponse(res, "running")
}

func (b3 *BigBlueButton) End(id, password string) bool {
	u := b3.makeURL("end", url.Values{"meetingID": {id}, "password": {password}})
	if _, err := http.Get(u.String()); nil == err {
		for retries := 0; retries < 10; retries++ {
			if _, err := b3.MeetingInfo(id, password); nil != err {
				return true
			}
			time.Sleep(2 * time.Second)
		}
	}
	return false
}

func (b3 *BigBlueButton) MeetingInfo(id, password string) (*Meeting, error) {
	u := b3.makeURL("getMeetingInfo", url.Values{"meetingID": {id}, "password": {password}})
	res, err := http.Get(u.String())
	if nil != err {
		return nil, err
	}
	defer res.Body.Close()
	return loadMeetingInfoResponse(res)
}

func (b3 *BigBlueButton) Meetings() []*Meeting {
	u := b3.makeURL("getMeetings", url.Values{})
	res, err := http.Get(u.String())
	if nil != err {
		return []*Meeting{}
	}
	defer res.Body.Close()
	return loadMeetigsResponse(res)
}

func (b3 *BigBlueButton) Recordings(meetings []string) []*Recording {
	q := url.Values{}
	if len(meetings) > 0 {
		q.Set("meetingID", strings.Join(meetings, ","))
	}
	u := b3.makeURL("getRecordings", q)
	res, err := http.Get(u.String())
	if nil != err {
		return []*Recording{}
	}
	defer res.Body.Close()
	return loadRecordingsResponse(res)
}

func (b3 *BigBlueButton) PublishRecordings(recordings []string, publish bool) bool {
	if len(recordings) > 0 {
		u := b3.makeURL("publishRecordings", url.Values{
			"recordID": {strings.Join(recordings, ",")},
			"publish":  {strconv.FormatBool(publish)},
		})
		res, err := http.Get(u.String())
		if nil != err {
			return false
		}
		defer res.Body.Close()
		return loadBoolResponse(res, "published")
	}
	return false
}

func (b3 *BigBlueButton) DeleteRecordings(recordings []string) bool {
	if len(recordings) > 0 {
		u := b3.makeURL("deleteRecordings", url.Values{
			"recordID": {strings.Join(recordings, ",")},
		})
		res, err := http.Get(u.String())
		if nil != err {
			return false
		}
		defer res.Body.Close()
		return loadBoolResponse(res, "deleted")
	}
	return false
}

func (b3 *BigBlueButton) ServerVersion() string {
	res, err := http.Get(b3.Url.String())
	if nil != err {
		return ""
	}
	defer res.Body.Close()
	return loadStringResponse(res, "version")
}

func (b3 *BigBlueButton) checksum(action, params string) string {
	if i := len(params) - 1; i > 0 && params[i] == '&' {
		params = params[:i]
	}
	h := sha1.New()
	io.WriteString(h, action)
	io.WriteString(h, params)
	io.WriteString(h, b3.Secret)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (b3 *BigBlueButton) makeURL(action string, query url.Values) *url.URL {
	if _, t := query["checksum"]; !t {
		query.Add("checksum", b3.checksum(action, query.Encode()))
	}
	u, _ := b3.Url.Parse(action + "?" + query.Encode())
	return u
}

func mergeUrlValues(values ...url.Values) (m url.Values) {
	m = url.Values{}
	for _, v := range values {
		for k, v := range v {
			if _, t := m[k]; t {
				m[k] = append(m[k], v...)
			} else {
				m[k] = v
			}
		}
	}
	return
}
