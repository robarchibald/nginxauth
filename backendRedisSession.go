package main

import (
	"github.com/pkg/errors"
	"github.com/robarchibald/onedb"
	"math"
	"time"
)

type backendRedisSession struct {
	db     onedb.DBer
	prefix string
}

func newBackendRedisSession(server string, port int, password string, maxIdle, maxConnections int, keyPrefix string) sessionBackender {
	r := onedb.NewRedis(server, port, password, maxIdle, maxConnections)
	return &backendRedisSession{db: r, prefix: keyPrefix}
}

// need to first check that this emailVerifyHash isn't being used, otherwise we'll clobber existing
func (r *backendRedisSession) CreateEmailSession(email, emailVerifyHash, destinationURL string) error {
	return r.UpdateEmailSession(emailVerifyHash, -1, email, destinationURL)
}

func (r *backendRedisSession) GetEmailSession(emailVerifyHash string) (*emailSession, error) {
	session := &emailSession{}
	return session, r.db.QueryStructRow(onedb.NewRedisGetCommand(r.getEmailSessionKey(emailVerifyHash)), session)
}

func (r *backendRedisSession) UpdateEmailSession(emailVerifyHash string, userID int, email, destinationURL string) error {
	return r.saveEmailSession(&emailSession{userID, email, emailVerifyHash, destinationURL})
}

func (r *backendRedisSession) DeleteEmailSession(emailVerifyHash string) error {
	return nil
}

func (r *backendRedisSession) CreateSession(userID int, email, fullname, sessionHash string, sessionRenewTimeUTC, sessionExpireTimeUTC time.Time,
	includeRememberMe bool, rememberMeSelector, rememberMeTokenHash string, rememberMeRenewTimeUTC, rememberMeExpireTimeUTC time.Time) (*loginSession, *rememberMeSession, error) {
	session := loginSession{userID, email, fullname, sessionHash, sessionRenewTimeUTC, sessionExpireTimeUTC}
	err := r.saveSession(&session)
	if err != nil {
		return nil, nil, err
	}

	var rememberMe rememberMeSession
	if includeRememberMe {
		rememberMe = rememberMeSession{userID, email, rememberMeSelector, rememberMeTokenHash, rememberMeRenewTimeUTC, rememberMeExpireTimeUTC}
		err = r.saveRememberMe(&rememberMe)
		if err != nil {
			return nil, nil, err
		}
	}

	return &session, &rememberMe, nil
}

func (r *backendRedisSession) GetSession(sessionHash string) (*loginSession, error) {
	session := &loginSession{}
	return session, r.db.QueryStructRow(onedb.NewRedisGetCommand(r.getSessionKey(sessionHash)), session)
}

func (r *backendRedisSession) RenewSession(sessionHash string, renewTimeUTC time.Time) (*loginSession, error) {
	session, err := r.GetSession(sessionHash)
	if err != nil {
		return nil, err
	}
	if session.ExpireTimeUTC.Before(time.Now().UTC()) {
		session.ExpireTimeUTC = time.Now().UTC().Add(sessionExpireDuration)
	}
	session.RenewTimeUTC = renewTimeUTC
	if err := r.saveSession(session); err != nil {
		return nil, err
	}
	return session, nil
}

func (r *backendRedisSession) InvalidateSession(sessionHash string) error {
	return r.db.Execute(onedb.NewRedisDelCommand(r.getSessionKey(sessionHash)))
}

func (r *backendRedisSession) InvalidateSessions(email string) error {
	return nil
}

func (r *backendRedisSession) GetRememberMe(selector string) (*rememberMeSession, error) {
	rememberMe := &rememberMeSession{}
	return rememberMe, r.db.QueryStructRow(onedb.NewRedisGetCommand(r.getRememberMeKey(selector)), rememberMe)
}

func (r *backendRedisSession) RenewRememberMe(selector string, renewTimeUTC time.Time) (*rememberMeSession, error) {
	rememberMe, err := r.GetRememberMe(selector)
	if err != nil {
		return nil, err
	}
	rememberMe.RenewTimeUTC = renewTimeUTC
	if err := r.saveRememberMe(rememberMe); err != nil {
		return nil, err
	}
	return rememberMe, nil
}

func (r *backendRedisSession) InvalidateRememberMe(selector string) error {
	return r.db.Execute(onedb.NewRedisDelCommand(r.getRememberMeKey(selector)))
}

func (r *backendRedisSession) Close() error {
	return r.db.Close()
}

func (r *backendRedisSession) saveEmailSession(session *emailSession) error {
	return r.save(r.getEmailSessionKey(session.EmailVerifyHash), session, emailExpireMins*60)
}

func (r *backendRedisSession) saveSession(session *loginSession) error {
	if time.Since(session.ExpireTimeUTC).Seconds() >= 0 {
		return errors.New("Unable to save expired session")
	}
	return r.save(r.getSessionKey(session.SessionHash), session, round(rememberMeExpireDuration.Seconds()))
}

func (r *backendRedisSession) saveRememberMe(rememberMe *rememberMeSession) error {
	if time.Since(rememberMe.ExpireTimeUTC).Seconds() >= 0 {
		return errors.New("Unable to save expired rememberMe")
	}
	return r.save(r.getRememberMeKey(rememberMe.Selector), rememberMe, round(rememberMeExpireDuration.Seconds()))
}

func (r *backendRedisSession) getEmailSessionKey(emailVerifyHash string) string {
	return r.prefix + "/email/" + emailVerifyHash
}

func (r *backendRedisSession) getSessionKey(sessionHash string) string {
	return r.prefix + "/session/" + sessionHash
}

func (r *backendRedisSession) getRememberMeKey(selector string) string {
	return r.prefix + "/rememberMe/" + selector
}

func round(num float64) int {
	return int(math.Floor(0.5 + num))
}

func (r *backendRedisSession) save(key string, value interface{}, expireSeconds int) error {
	cmd, err := onedb.NewRedisSetCommand(key, value, expireSeconds)
	if err != nil {
		return err
	}
	return r.db.Execute(cmd)
}
