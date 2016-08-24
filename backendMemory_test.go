package nginxauth

import (
	"testing"
	"time"
)

var in5Minutes time.Time = time.Now().UTC().Add(5 * time.Minute)
var in1Hour time.Time = time.Now().UTC().Add(time.Hour)

func TestBackendGetUserLogin(t *testing.T) {
	backend := NewBackendMemory()
	if _, err := backend.GetUserLogin("email", LoginProviderDefaultName); err != ErrUserNotFound {
		t.Error("expected no login since nothing added yet", err)
	}
	backend.Users = append(backend.Users, &User{PrimaryEmail: "email", UserId: 1})
	if _, err := backend.GetUserLogin("email", LoginProviderDefaultName); err != ErrLoginNotFound {
		t.Error("expected no login since login not added yet", err)
	}
	expected := &UserLogin{UserId: 1, LoginProviderId: 1}
	backend.Logins = append(backend.Logins, expected)
	if actual, _ := backend.GetUserLogin("email", LoginProviderDefaultName); expected != actual {
		t.Error("expected no login since login not added yet")
	}
}

func TestBackendNewLoginSession(t *testing.T) {
	backend := NewBackendMemory()
	if _, _, err := backend.NewLoginSession(1, "sessionId", in5Minutes, in1Hour, false, "", "", time.Time{}, time.Time{}); err != ErrLoginNotFound {
		t.Error("expected error since login doesn't exist")
	}
	backend.Logins = append(backend.Logins, &UserLogin{UserId: 1, LoginId: 1})
	if session, _, _ := backend.NewLoginSession(1, "sessionId", in5Minutes, in1Hour, false, "", "", time.Time{}, time.Time{}); session.SessionId != "sessionId" || session.IsHalfAuth != false || session.LoginId != 1 || session.UserId != 1 {
		t.Error("expected matching session", session)
	}

	// existing remember already exists
	backend.RememberMes = append(backend.RememberMes, &UserLoginRememberMe{LoginId: 1, Selector: "selector"})
	if session, rememberMe, err := backend.NewLoginSession(1, "sessionId", in5Minutes, in1Hour, true, "selector", "hash", time.Time{}, time.Time{}); session.SessionId != "sessionId" ||
		session.IsHalfAuth != false || session.LoginId != 1 || session.UserId != 1 ||
		rememberMe.LoginId != 1 || rememberMe.Selector != "selector" || rememberMe.TokenHash != "hash" {
		t.Error("expected RememberMe to be created", session, rememberMe, err)
	}

	// create new rememberMe
	if session, rememberMe, err := backend.NewLoginSession(1, "sessionId", in5Minutes, in1Hour, true, "newselector", "hash", time.Time{}, time.Time{}); session.SessionId != "sessionId" ||
		session.IsHalfAuth != false || session.LoginId != 1 || session.UserId != 1 ||
		rememberMe.LoginId != 1 || rememberMe.Selector != "newselector" || rememberMe.TokenHash != "hash" {
		t.Error("expected RememberMe to be created", session, rememberMe, err)
	}

	// existing remember is for different login... error
	backend.RememberMes = append(backend.RememberMes, &UserLoginRememberMe{LoginId: 2, Selector: "otherselector"})
	if _, _, err := backend.NewLoginSession(1, "sessionId", in5Minutes, in1Hour, true, "otherselector", "hash", time.Time{}, time.Time{}); err != ErrRememberMeSelectorExists {
		t.Error("expected error", err)
	}
}

func TestBackendGetSession(t *testing.T) {
	backend := NewBackendMemory()
	if _, err := backend.GetSession("sessionId"); err != ErrSessionNotFound {
		t.Error("expected err", err)
	}

	// add session now and try again... should be returned
	backend.Sessions = append(backend.Sessions, &UserLoginSession{SessionId: "sessionId"})
	if session, _ := backend.GetSession("sessionId"); session.SessionId != "sessionId" {
		t.Error("expected session to be returned", session)
	}
}

func TestBackendRenewSession(t *testing.T) {
	backend := NewBackendMemory()
	renews := time.Now()
	if _, err := backend.RenewSession("sessionId", renews); err != ErrSessionNotFound {
		t.Error("expected err", err)
	}

	// add session now and try again... should be renewed
	backend.Sessions = append(backend.Sessions, &UserLoginSession{SessionId: "sessionId"})
	if session, _ := backend.RenewSession("sessionId", renews); session.SessionId != "sessionId" || session.RenewsAt != renews {
		t.Error("expected session to be renewed", session)
	}
}

func TestBackendGetRememberMe(t *testing.T) {
	backend := NewBackendMemory()
	if _, err := backend.GetRememberMe("selector"); err != ErrRememberMeNotFound {
		t.Error("expected err", err)
	}

	// add rememberMe now and try again... should be returned
	backend.RememberMes = append(backend.RememberMes, &UserLoginRememberMe{Selector: "selector"})
	if rememberMe, _ := backend.GetRememberMe("selector"); rememberMe.Selector != "selector" {
		t.Error("expected rememberMe to be found", rememberMe)
	}
}

func TestBackendRenewRememberMe(t *testing.T) {
	backend := NewBackendMemory()
	renews := time.Now().UTC().Add(5 * time.Minute)
	if _, err := backend.RenewRememberMe("selector", renews); err != ErrRememberMeNotFound {
		t.Error("expected err", err)
	}

	backend.RememberMes = append(backend.RememberMes, &UserLoginRememberMe{Selector: "expired", ExpiresAt: time.Now().UTC().Add(-1 * time.Hour)})
	if _, err := backend.RenewRememberMe("expired", renews); err != ErrRememberMeExpired {
		t.Error("expected expired", err)
	}

	backend.RememberMes = append(backend.RememberMes, &UserLoginRememberMe{Selector: "selector", ExpiresAt: time.Now().UTC().Add(time.Hour)})
	if _, err := backend.RenewRememberMe("selector", time.Now().UTC().Add(2*time.Hour)); err != ErrInvalidRenewsAtTime {
		t.Error("expected invalid renew time", err)
	}
	if _, err := backend.RenewRememberMe("selector", time.Now().UTC().Add(-1*time.Hour)); err != ErrInvalidRenewsAtTime {
		t.Error("expected invalid renew time", err)
	}
	if rememberMe, _ := backend.RenewRememberMe("selector", renews); rememberMe.RenewsAt != renews {
		t.Error("expected valid rememberMe", rememberMe)
	}
}

func TestBackendAddUser(t *testing.T) {
	backend := NewBackendMemory()
	renews := time.Now().UTC()
	expires := time.Now().UTC().Add(time.Hour)
	if session, _ := backend.AddUser("email", "emailVerifyHash", "sessionId", renews, expires); session.UserId != 1 || session.SessionId != "sessionId" || session.ExpiresAt != expires || session.RenewsAt != renews || session.IsHalfAuth != true || session.LoginId != 1 {
		t.Error("expected valid session", session)
	}

	if _, err := backend.AddUser("email", "emailVerifyHash", "sessionId", renews, expires); err != ErrUserAlreadyExists {
		t.Error("expected user to already exist", err)
	}

	if _, err := backend.AddUser("email1", "emailVerifyHash", "sessionId", renews, expires); err != ErrEmailVerifyCodeExists {
		t.Error("expected failure due to existing email verify code", err)
	}
}

func TestBackendVerifyEmail(t *testing.T) {
	backend := NewBackendMemory()
	renews := time.Now().UTC()
	expires := time.Now().UTC().Add(time.Hour)
	if _, _, err := backend.VerifyEmail("verifyHash", "sessionId", renews, expires); err != ErrInvalidEmailVerifyCode {
		t.Error("expected login not found err", err)
	}

	backend.Users = append(backend.Users, &User{EmailVerifyHash: "verifyHash", UserId: 1, PrimaryEmail: "email"})
	if _, _, err := backend.VerifyEmail("verifyHash", "sessionId", renews, expires); err != ErrLoginNotFound {
		t.Error("expected login not found err", err)
	}
	backend.Logins = append(backend.Logins, &UserLogin{UserId: 1, LoginId: 1, LoginProviderId: 1})
	if session, email, _ := backend.VerifyEmail("verifyHash", "sessionId", renews, expires); session.SessionId != "sessionId" || session.RenewsAt != renews || session.ExpiresAt != expires || session.LoginId != 1 || session.UserId != 1 || session.IsHalfAuth != true || email != "email" {
		t.Error("expected valid session", session)
	}

	// run it again.  Should just update the renew and expire time
	if session, email, _ := backend.VerifyEmail("verifyHash", "sessionId", renews.Add(time.Minute), expires.Add(time.Minute)); session.SessionId != "sessionId" || session.RenewsAt != renews.Add(time.Minute) || session.ExpiresAt != expires.Add(time.Minute) || session.LoginId != 1 || session.UserId != 1 || session.IsHalfAuth != true || email != "email" {
		t.Error("expected valid session", session)
	}
	// invalid session.  Doesn't have the same loginId and UserId of the hash
	backend.Sessions = append(backend.Sessions, &UserLoginSession{LoginId: 2, UserId: 2, SessionId: "invalidSession"})
	if _, _, err := backend.VerifyEmail("verifyHash", "invalidSession", renews, expires); err != ErrInvalidSessionId {
		t.Error("expected valid session", session)
	}
}

func TestBackendUpdateUser(t *testing.T) {
	backend := NewBackendMemory()
	backend.UpdateUser(nil, "fullname", "company", "pictureUrl")
}

func TestBackendCreateProfileAndInvalidateSessions(t *testing.T) {
	renews := time.Now().UTC()
	expires := time.Now().UTC().Add(time.Hour)
	backend := NewBackendMemory()
	if _, err := backend.CreateProfileAndInvalidateSessions(1, "passwordHash", "fullName", "company", "pictureUrl", "sessionId", expires, renews); err != ErrLoginNotFound {
		t.Error("expected login not found err", err)
	}

	backend.Logins = append(backend.Logins, &UserLogin{UserId: 1, LoginId: 1, LoginProviderId: 1})
	if _, err := backend.CreateProfileAndInvalidateSessions(1, "passwordHash", "fullName", "company", "pictureUrl", "sessionId", expires, renews); err != ErrUserNotFound {
		t.Error("expected user not found err", err)
	}

	backend.Users = append(backend.Users, &User{EmailVerifyHash: "verifyHash", UserId: 1, PrimaryEmail: "email"})
	if session, _ := backend.CreateProfileAndInvalidateSessions(1, "passwordHash", "fullName", "company", "pictureUrl", "sessionId", expires, renews); session.SessionId != "sessionId" || session.ExpiresAt != expires || session.RenewsAt != renews || session.LoginId != 1 || session.UserId != 1 || session.IsHalfAuth != false {
		t.Error("expected valid session", session)
	}
}

func TestToString(t *testing.T) {
	backend := NewBackendMemory()
	backend.Users = append(backend.Users, &User{})
	backend.Logins = append(backend.Logins, &UserLogin{})
	backend.Sessions = append(backend.Sessions, &UserLoginSession{})
	backend.RememberMes = append(backend.RememberMes, &UserLoginRememberMe{})

	actual := backend.ToString()
	expected := "Users:\n     {0    false <nil> 0}\nLogins:\n     {0 0 0 }\nSessions:\n     {0  0 0001-01-01 00:00:00 +0000 UTC 0001-01-01 00:00:00 +0000 UTC false}\nRememberMe:\n     {0   0001-01-01 00:00:00 +0000 UTC 0001-01-01 00:00:00 +0000 UTC}\n"
	if actual != expected {
		t.Error("expected different value", actual)
	}
}

func TestGetLoginProvider(t *testing.T) {
	backend := NewBackendMemory()
	if backend.getLoginProvider("bogus") != nil {
		t.Error("expected no provider")
	}
}

func TestGetLoginByUser(t *testing.T) {
	backend := NewBackendMemory()
	if backend.getLoginByUser(1, "bogus") != nil {
		t.Error("expected no login")
	}
}
