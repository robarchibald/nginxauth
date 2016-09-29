package nginxauth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/securecookie"
)

var cookieKey []byte = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 38, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64}

func getCookieStore() *CookieStore {
	r, _ := http.NewRequest("GET", "www.google.com", nil)
	return NewCookieStore(httptest.NewRecorder(), r, cookieKey, false)
}

func TestNewCookieStore(t *testing.T) {
	r := &http.Request{}
	w := httptest.NewRecorder()
	actual := NewCookieStore(w, r, cookieKey, false)
	if actual.w != w || actual.r != r {
		t.Fatal("expected correct init", actual)
	}
}

func TestGetCookie(t *testing.T) {
	store := getCookieStore()
	renewsTimeUTC := time.Date(2001, 1, 1, 12, 0, 0, 0, time.Local)
	expiresTimeUTC := time.Date(2002, 1, 1, 12, 0, 0, 0, time.Local)
	value, err := securecookie.New(cookieKey, nil).Encode("myCookie", &SessionCookie{"sessionId", renewsTimeUTC, expiresTimeUTC})
	store.r.AddCookie(&http.Cookie{Expires: time.Date(2001, 1, 1, 12, 0, 0, 0, time.Local), Name: "myCookie", Value: value})

	cookie := SessionCookie{}
	err = store.Get("myCookie", &cookie)
	if err != nil || cookie.ExpireTimeUTC != expiresTimeUTC || cookie.RenewTimeUTC != renewsTimeUTC || cookie.SessionId != "sessionId" {
		t.Fatal("unexpected", err, cookie)
	}
}

func TestGetCookieBogusValue(t *testing.T) {
	store := getCookieStore()
	store.r.AddCookie(&http.Cookie{Expires: time.Date(2001, 1, 1, 12, 0, 0, 0, time.Local), Name: "myCookie", Value: "bogus"})

	cookie := SessionCookie{}
	err := store.Get("myCookie", &cookie)
	if err == nil {
		t.Fatal("expected fail")
	}
}

func TestGetCookieMissing(t *testing.T) {
	store := getCookieStore()

	cookie := SessionCookie{}
	err := store.Get("myCookie", &cookie)
	if err == nil {
		t.Fatal("expected fail")
	}
}

func TestPutCookie(t *testing.T) {
	store := getCookieStore()
	renewTimeUTC := time.Date(2001, 1, 1, 12, 0, 0, 0, time.Local)
	expireTimeUTC := time.Date(2002, 1, 1, 12, 0, 0, 0, time.Local)
	expectedCookieExpiration := time.Now().AddDate(0, 1, 1) // add 1 month to today
	expected := &SessionCookie{"sessionId", renewTimeUTC, expireTimeUTC}
	err := store.Put("myCookie", expected)

	rawCookie := store.w.Header().Get("Set-Cookie")
	name := rawCookie[0:strings.Index(rawCookie, "=")]
	value := substringBetween(rawCookie, "=", "; ")
	actual := SessionCookie{}
	securecookie.New(cookieKey, nil).Decode("myCookie", value, &actual)
	path := substringBetween(rawCookie, "Path=", ";")
	expires := substringBetween(rawCookie, "Expires=", ";")
	maxAge := substringBetween(rawCookie, "Max-Age=", ";")
	expireTime, err := time.Parse("Mon, 02 Jan 2006 15:04:05 MST", expires)
	if err != nil || name != "myCookie" || actual.SessionId != expected.SessionId ||
		actual.ExpireTimeUTC != expected.ExpireTimeUTC || actual.RenewTimeUTC != expected.RenewTimeUTC ||
		path != "/" || maxAge != "43200" || expireTime.Sub(expectedCookieExpiration) > 1*time.Second {
		t.Fatal("unexpected", err, name, path, expireTime, maxAge)
	}
}

func TestPutCookieBogus(t *testing.T) {
	store := getCookieStore()
	err := store.Put(";;;aa9083a09vdad", "11111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")

	if err == nil {
		t.Fatal("expected fail")
	}
}

func substringBetween(source, from, to string) string {
	fromIndex := strings.Index(source, from) + len(from)
	toIndex := strings.Index(source[fromIndex:], to) + fromIndex
	return source[fromIndex:toIndex]
}

/****************************************************************************/
type MockCookieStore struct {
	CookieStorer
	cookies map[string]interface{}
	getErr  error
	putErr  error
}

func NewMockCookieStore(cookies map[string]interface{}, hasGetErr, hasPutErr bool) *MockCookieStore {
	err := errors.New("failed")
	var getErr, putErr error
	if hasGetErr {
		getErr = err
	}
	if hasPutErr {
		putErr = err
	}
	return &MockCookieStore{cookies: cookies, getErr: getErr, putErr: putErr}
}

func (c *MockCookieStore) Get(key string, result interface{}) error {
	val, ok := c.cookies[key]
	if c.getErr != nil || !ok || val == nil || reflect.ValueOf(val) == reflect.Zero(reflect.TypeOf(val)) {
		return c.getErr
	}
	resultVal := reflect.ValueOf(result).Elem()
	itemVal := reflect.ValueOf(c.cookies[key]).Elem()
	for i := 0; i < itemVal.NumField(); i++ {
		resultVal.Field(i).Set(itemVal.Field(i))
	}
	return c.getErr
}

func (c *MockCookieStore) PutWithExpire(key string, expire int, value interface{}) error {
	c.cookies[key] = value
	return c.putErr
}

func (c *MockCookieStore) Put(key string, value interface{}) error {
	return c.PutWithExpire(key, 150, value)
}

func (c *MockCookieStore) Delete(key string) {
	c.cookies[key] = nil
}

func rememberCookie(renewTimeUTC, expireTimeUTC time.Time) *RememberMeCookie {
	return &RememberMeCookie{"selector", "dG9rZW4=", renewTimeUTC, expireTimeUTC} // dG9rZW4= is base64 encode of "token"
}

func sessionCookie(renewTimeUTC, expireTimeUTC time.Time) *SessionCookie {
	return &SessionCookie{"nfwRDzfxxJj2_HY-_mLz6jWyWU7bF0zUlIUUVkQgbZ0=", renewTimeUTC, expireTimeUTC}
}

func sessionBogusCookie(renewTimeUTC, expireTimeUTC time.Time) *SessionCookie {
	return &SessionCookie{"sessionId", renewTimeUTC, expireTimeUTC}
}
