package nginxauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type SessionStorer interface {
	GetSession() (*UserLoginSession, error)
	GetBasicAuth() (*UserLoginSession, error)
	Login() error
	Register() error
	CreateProfile() error
	VerifyEmail() error
	UpdateEmail() error
	UpdatePassword() error
}

type SessionCookie struct {
	SessionId string
	RenewsAt  time.Time
	ExpiresAt time.Time
}

type RememberMeCookie struct {
	Selector  string
	Token     string
	RenewsAt  time.Time
	ExpiresAt time.Time
}

type SessionStore struct {
	backend     BackendQuerier
	mailer      Mailer
	cookieStore CookieStorer
	r           *http.Request
}

var emailRegex = regexp.MustCompile(`^(?i)[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}$`)

func NewSessionStore(backend BackendQuerier, mailer Mailer, w http.ResponseWriter, r *http.Request, cookieKey []byte, cookiePrefix string) *SessionStore {
	sessionCookieName = cookiePrefix + "Session"
	rememberMeCookieName = cookiePrefix + "RememberMe"
	return &SessionStore{backend, mailer, NewCookieStore(w, r, cookieKey), r}
}

var sessionCookieName string = "Session"
var rememberMeCookieName string = "RememberMe"

const sessionRenewDuration time.Duration = 5 * time.Minute
const sessionExpireDuration time.Duration = time.Hour
const rememberMeRenewDuration time.Duration = time.Hour
const rememberMeExpireDuration time.Duration = time.Hour * 24 * 30

func (s *SessionStore) GetSession() (*UserLoginSession, error) {
	cookie, err := s.getSessionCookie()
	if err != nil { // impossible to get the session if there is no cookie
		return nil, NewAuthError("Session cookie not found", err)
	}

	if cookie.RenewsAt.Before(time.Now().UTC()) || cookie.ExpiresAt.Before(time.Now().UTC()) {
		return s.renewSession(cookie.SessionId, &cookie.RenewsAt, &cookie.ExpiresAt)
	}

	session, err := s.backend.GetSession(cookie.SessionId)
	if err != nil {
		if err == ErrSessionNotFound {
			s.deleteSessionCookie()
		}
		return nil, NewLoggedError("Failed to verify session", err)
	}
	return session, nil
}

func (s *SessionStore) GetBasicAuth() (*UserLoginSession, error) {
	session, err := s.GetSession()
	if err != nil {
		if email, password, ok := s.r.BasicAuth(); ok {
			session, err = s.login(email, password, false)
			if err != nil {
				return nil, NewLoggedError("Unable to login with provided credentials", err)
			}
		} else {
			return nil, NewAuthError("Problem decoding credentials from basic auth", nil)
		}
	}
	return session, nil
}

func (s *SessionStore) getRememberMe() (*UserLoginRememberMe, error) {
	cookie, err := s.getRememberMeCookie()
	if err != nil { // impossible to get the remember Me if there is no cookie
		return nil, NewAuthError("RememberMe cookie not found", err)
	}
	if cookie.ExpiresAt.Before(time.Now().UTC()) {
		s.deleteRememberMeCookie()
		return nil, NewAuthError("RememberMe cookie has expired", nil)
	}

	rememberMe, err := s.backend.GetRememberMe(cookie.Selector)
	if err != nil {
		if err == ErrRememberMeNotFound {
			s.deleteRememberMeCookie()
		}
		return nil, NewLoggedError("Unable to find matching RememberMe in DB", err)
	}
	if !encodedHashEquals(cookie.Token, rememberMe.TokenHash) {
		s.deleteRememberMeCookie()
		return nil, NewLoggedError("RememberMe cookie doesn't match backend token", nil)
	}
	if rememberMe.RenewsAt.Before(time.Now().UTC()) {
		rememberMe, err = s.backend.RenewRememberMe(cookie.Selector, time.Now().UTC().Add(rememberMeRenewDuration))
		if err != nil {
			if err == ErrRememberMeNotFound {
				s.deleteRememberMeCookie()
			}
			return nil, NewLoggedError("Unable to renew RememberMe", err)
		}
	}
	return rememberMe, nil
}

func (s *SessionStore) renewSession(sessionId string, renewsAt, expiresAt *time.Time) (*UserLoginSession, error) {
	var session *UserLoginSession
	var err error
	if renewsAt.Before(time.Now().UTC()) && expiresAt.After(time.Now().UTC()) {
		session, err := s.backend.RenewSession(sessionId, time.Now().UTC().Add(sessionRenewDuration))
		if err != nil {
			return nil, NewLoggedError("Unable to renew session", err)
		}

		if err = s.saveSessionCookie(session.SessionId, session.RenewsAt, session.ExpiresAt); err != nil {
			return nil, err
		}
		return session, nil
	}

	_, err = s.getRememberMe()
	if err != nil {
		return nil, NewAuthError("Unable to renew session", err)
	}

	session, err = s.backend.RenewSession(sessionId, time.Now().UTC().Add(sessionRenewDuration))
	if err != nil {
		if err == ErrSessionNotFound {
			s.deleteSessionCookie()
		}
		return nil, NewLoggedError("Problem renewing session", err)
	}

	if err = s.saveSessionCookie(session.SessionId, session.RenewsAt, session.ExpiresAt); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *SessionStore) Login() error {
	credentials, err := getCredentials(s.r)
	if err != nil {
		return NewAuthError("Unable to get credentials", err)
	}
	_, err = s.login(credentials.Email, credentials.Password, credentials.RememberMe)
	return err
}

func (s *SessionStore) login(email, password string, rememberMe bool) (*UserLoginSession, error) {
	if !isValidEmail(email) {
		return nil, NewAuthError("Please enter a valid email address.", nil)
	}
	if !isValidPassword(password) {
		return nil, NewAuthError(passwordValidationMessage, nil)
	}

	login, err := s.backend.GetUserLogin(email, LoginProviderDefaultName)
	if err != nil {
		return nil, NewLoggedError("Invalid username or password", err)
	}

	decoded, _ := decodeFromString(login.ProviderKey)
	if !hashEquals([]byte(password), decoded) {
		return nil, NewLoggedError("Invalid username or password", nil)
	}

	return s.createSession(login.LoginId, rememberMe)
}

func (s *SessionStore) createSession(loginId int, rememberMe bool) (*UserLoginSession, error) {
	var err error
	var selector, token, tokenHash string
	if rememberMe {
		selector, token, tokenHash, err = generateSelectorTokenAndHash()
		if err != nil {
			return nil, NewLoggedError("Unable to generate RememberMe", err)
		}
	}
	sessionId, err := generateRandomString()
	if err != nil {
		return nil, NewLoggedError("Problem generating sessionId", nil)
	}

	session, remember, err := s.backend.NewLoginSession(loginId, sessionId, time.Now().UTC().Add(sessionRenewDuration), time.Now().UTC().Add(sessionExpireDuration), rememberMe, selector, tokenHash, time.Now().UTC().Add(rememberMeRenewDuration), time.Now().UTC().Add(rememberMeExpireDuration))
	if err != nil {
		return nil, NewLoggedError("Unable to create new session", err)
	}

	cookie, err := s.getSessionCookie()
	if err == nil && cookie != nil {
		// remove existing session from backend
	}

	if rememberMe {
		err := s.saveRememberMeCookie(selector, token, remember.RenewsAt, remember.ExpiresAt)
		if err != nil {
			return nil, NewAuthError("Unable to save rememberMe cookie", err)
		}
	}
	err = s.saveSessionCookie(session.SessionId, session.RenewsAt, session.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

type SendVerifyParams struct {
	VerificationCode string
	Email            string
	RefererBaseUrl   string
}

func (s *SessionStore) Register() error {
	registration, err := getRegistration(s.r)
	if err != nil {
		return NewAuthError("Unable to get email", err)
	}
	return s.register(registration.Email)
}

func (s *SessionStore) register(email string) error {
	if !isValidEmail(email) {
		return NewAuthError("Invalid email", nil)
	}

	session, emailConfirmCode, err := s.addUser(email)
	if err != nil {
		return NewLoggedError("Unable to save user", err)
	}

	err = s.saveSessionCookie(session.SessionId, session.RenewsAt, session.ExpiresAt)
	if err != nil {
		return err
	}
	code := emailConfirmCode[:len(emailConfirmCode)-1] // drop the "=" at the end of the code
	if err := s.mailer.SendVerify(email, &SendVerifyParams{code, email, getBaseUrl(s.r.Referer())}); err != nil {
		return NewLoggedError("Unable to send verification email", err)
	}

	return nil
}

func getBaseUrl(url string) string {
	protoIndex := strings.Index(url, "://")
	if protoIndex == -1 { // must be relative URL, return default
		return "https://endfirst.com"
	}
	firstSlash := strings.Index(url[protoIndex+3:], "/")
	if firstSlash == -1 { // must not have trailing slash, return whole url
		return url
	}
	return url[:protoIndex+3+firstSlash]
}

func (s *SessionStore) addUser(email string) (*UserLoginSession, string, error) {
	emailConfirmCode, emailConfimHash, err := generateStringAndHash()
	if err != nil {
		return nil, "", NewLoggedError("Problem generating email confirmation code", err)
	}

	sessionId, err := generateRandomString()
	if err != nil {
		return nil, "", NewLoggedError("Problem generating sessionId", err)
	}

	session, err := s.backend.AddUser(email, emailConfimHash, sessionId, time.Now().UTC().Add(sessionRenewDuration), time.Now().UTC().Add(sessionExpireDuration))
	if err != nil {
		return nil, "", NewLoggedError("Problem adding user to database", err)
	}
	return session, emailConfirmCode, nil
}

func (s *SessionStore) CreateProfile() error {
	profile, err := getProfile(s.r)
	if err != nil {
		return NewAuthError("Unable to get profile information from form", err)
	}
	return s.createProfile(profile.FullName, profile.Organization, profile.Password, profile.PicturePath)
}

func (s *SessionStore) createProfile(fullName, organization, password, picturePath string) error {
	session, err := s.GetSession() // verify the user has access
	if err != nil {
		return NewLoggedError("Unable to get session", err)
	}

	passwordHash := encodeToString(hash([]byte(password)))
	session, err = s.backend.CreateProfileAndInvalidateSessions(session.LoginId, passwordHash, fullName, organization, picturePath, session.SessionId, session.ExpiresAt, session.RenewsAt)
	if err != nil {
		return NewLoggedError("Unable to create profile", err)
	}
	err = s.saveSessionCookie(session.SessionId, session.RenewsAt, session.ExpiresAt)
	if err != nil {
		return err
	}
	return nil
}

func (s *SessionStore) VerifyEmail() error {
	verify, err := getVerificationCode(s.r)
	if err != nil {
		return NewAuthError("Unable to get verification email from JSON", err)
	}
	return s.verifyEmail(verify.EmailVerificationCode)
}

func (s *SessionStore) verifyEmail(emailVerificationCode string) error {
	var sessionId string
	cookie, err := s.getSessionCookie()
	if err == nil { // use sessionId from current session
		sessionId = cookie.SessionId
	} else {
		sessionId, err = generateRandomString()
		if err != nil {
			return NewLoggedError("Unable to generate sessionId", err)
		}
	}
	data, err := decodeFromString(emailVerificationCode + "=") // add back the =, then decode
	if err != nil {
		return NewLoggedError("Invalid verification code", err)
	}
	emailVerifyHash := encodeToString(hash(data))
	session, email, err := s.backend.VerifyEmail(emailVerifyHash, sessionId, time.Now().UTC().Add(sessionRenewDuration), time.Now().Add(sessionExpireDuration))
	if err != nil {
		return NewLoggedError("Failed to verify email", err)
	}

	err = s.saveSessionCookie(session.SessionId, session.RenewsAt, session.ExpiresAt)
	if err != nil {
		return err
	}
	err = s.mailer.SendWelcome(email, nil)
	if err != nil {
		return NewLoggedError("Failed to send welcome email", err)
	}
	return nil
}

func (s *SessionStore) UpdateEmail() error { return nil }

func (s *SessionStore) UpdatePassword() error {
	return nil
}

func (s *SessionStore) getSessionCookie() (*SessionCookie, error) {
	session := &SessionCookie{}
	return session, s.cookieStore.Get(sessionCookieName, session)
}

func (s *SessionStore) getRememberMeCookie() (*RememberMeCookie, error) {
	rememberMe := &RememberMeCookie{}
	return rememberMe, s.cookieStore.Get(rememberMeCookieName, rememberMe)
}

func (s *SessionStore) deleteSessionCookie() {
	s.cookieStore.Delete(sessionCookieName)
}

func (s *SessionStore) deleteRememberMeCookie() {
	s.cookieStore.Delete(rememberMeCookieName)
}

func (s *SessionStore) saveSessionCookie(sessionId string, renewsAt, expiresAt time.Time) error {
	cookie := SessionCookie{SessionId: sessionId, RenewsAt: renewsAt, ExpiresAt: expiresAt}
	err := s.cookieStore.Put(sessionCookieName, &cookie)
	if err != nil {
		return NewAuthError("Error saving session cookie", err)
	}
	return nil
}

func (s *SessionStore) saveRememberMeCookie(selector, token string, renewsAt, expiresAt time.Time) error {
	cookie := RememberMeCookie{Selector: selector, Token: token, RenewsAt: renewsAt, ExpiresAt: expiresAt}
	return s.cookieStore.Put(rememberMeCookieName, &cookie)
}

type Registration struct {
	Email string
}

func getRegistration(r *http.Request) (*Registration, error) {
	register := &Registration{}
	return register, getJson(r, register)
}

type EmailVerificationCode struct {
	EmailVerificationCode string
}

func getVerificationCode(r *http.Request) (*EmailVerificationCode, error) {
	verificationCode := &EmailVerificationCode{}
	return verificationCode, getJson(r, verificationCode)
}

type Credentials struct {
	Email      string
	Password   string
	RememberMe bool
}

func getCredentials(r *http.Request) (*Credentials, error) {
	credentials := &Credentials{}
	return credentials, getJson(r, credentials)
}

func generateThumbnail(filename string) (string, error) {
	newName, err := generateRandomString()
	if err != nil {
		return "", NewLoggedError("Unable to create new filename", err)
	}
	var args = []string{
		"-s", "150",
		"-o", newName,
		filename,
	}

	var cmd *exec.Cmd
	path, _ := exec.LookPath("vipsthumbnail")
	cmd = exec.Command(path, args...)
	err = cmd.Run()
	if err != nil {
		return "", NewLoggedError("Error running vipsthumbnail", err)
	}
	return newName, nil
}

type Profile struct {
	FullName     string
	Organization string
	Password     string
	PicturePath  string
}

func getProfile(r *http.Request) (*Profile, error) {
	profile := &Profile{}
	r.ParseMultipartForm(32 << 20) // 32 MB file
	file, handler, err := r.FormFile("file")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	f, err := os.OpenFile(handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	io.Copy(f, file)

	profile.FullName = r.FormValue("fullName")
	profile.Organization = r.FormValue("Organization")
	profile.Password = r.FormValue("password")
	profile.PicturePath = handler.Filename

	return profile, nil
}

func getJson(r *http.Request, result interface{}) error {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

const passwordValidationMessage string = "Password must be between 7 and 20 characters"

func isValidPassword(password string) bool {
	return len(password) >= 7 && len(password) <= 20
}

func isValidEmail(email string) bool {
	return len(email) <= 254 && len(email) >= 6 && emailRegex.MatchString(email) == true
}

func decodeFromString(token string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(token)
}

func encodeToString(bytes []byte) string {
	return base64.URLEncoding.EncodeToString(bytes)
}

func generateSelectorTokenAndHash() (string, string, string, error) {
	var selector, token, tokenHash string
	selector, err := generateRandomString()
	if err != nil {
		return "", "", "", NewLoggedError("Unable to generate rememberMe selector", err)
	}
	token, tokenHash, err = generateStringAndHash()
	if err != nil {
		return "", "", "", NewLoggedError("Unable to generate rememberMe token", err)
	}
	return selector, token, tokenHash, nil
}

func generateStringAndHash() (string, string, error) {
	b, err := generateRandomBytes(32)
	if err != nil {
		return "", "", err
	}
	return encodeToString(b), encodeToString(hash(b)), nil
}

func hash(bytes []byte) []byte {
	h := sha256.Sum256(bytes)
	return h[:]
}

// Url decode both the token and the hash and then compare
func encodedHashEquals(token, tokenHash string) bool {
	tokenBytes, _ := decodeFromString(token)
	hashBytes, _ := decodeFromString(tokenHash)
	return hashEquals(tokenBytes, hashBytes)
}

func hashEquals(token, tokenHash []byte) bool {
	return subtle.ConstantTimeCompare(hash(token), tokenHash) == 1
}

func generateRandomString() (string, error) {
	bytes, err := generateRandomBytes(32)
	return encodeToString(bytes), err
}

func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}
