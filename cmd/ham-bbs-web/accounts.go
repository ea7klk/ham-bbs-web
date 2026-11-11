package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Account flows that need the shared token tables: activation codes, password
// resets and email changes, plus the per-address registration limit.
//
// The classic SSH BBS owns the design (bbs/cmd/bbs/tokens.go,
// bbs/cmd/bbs/account_mail.go). Everything here has to match it value for
// value, because either application may issue a token and the other may redeem
// it: a reset link mailed by the SSH BBS is opened in a browser, and an
// activation code written by this app is typed into a terminal. Only hashes are
// stored, so a copy of the database does not let anyone redeem a pending link.

const (
	// activationCodeLength is the number of characters in an activation code.
	activationCodeLength = 4
	// activationAlphabet omits I, O, 0 and 1: the code is read off a screen and
	// typed back, and those are the glyphs people transcribe wrongly.
	activationAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	// activationCodeTTL is how long a code stays valid.
	activationCodeTTL = 24 * time.Hour
	// activationAttempts is how many wrong codes are accepted before the code is
	// discarded and a new one must be requested.
	activationAttempts = 5

	// passwordResetTTL is how long a password-reset link works.
	passwordResetTTL = 2 * time.Hour
	// emailChangeTTL is how long an email-change link works.
	emailChangeTTL = 24 * time.Hour
	// tokenBytes is the entropy in a link token.
	tokenBytes = 32
	// tokenRetention is how long a spent or expired token row is kept, so an
	// operator can still see that a reset happened.
	tokenRetention = 30 * 24 * time.Hour
	// resetRequestsPerHour caps how many reset mails one account can trigger.
	resetRequestsPerHour = 3

	// registrationWindow is the period over which new accounts are counted.
	registrationWindow = time.Hour
	// registrationLimit is how many accounts one address may create per window.
	registrationLimit = 5
	// registrationRetention is how long an idle counter row is kept.
	registrationRetention = 24 * time.Hour
)

// Token purposes. The value is what both applications switch on.
const (
	tokenPurposePasswordReset = "password_reset"
	tokenPurposeEmailOld      = "email_change_old"
	tokenPurposeEmailNew      = "email_change_new"
)

var (
	errActivationExpired  = errors.New("activation code expired")
	errActivationTooMany  = errors.New("too many wrong activation codes")
	errActivationWrong    = errors.New("wrong activation code")
	errTokenInvalid       = errors.New("token is not valid")
	errRegistrationLimit  = errors.New("too many new accounts from this address")
	errAccountAlreadyLive = errors.New("account is already activated")
)

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// newToken returns a URL-safe token and its stored hash.
func newToken() (string, string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("token entropy: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

// newActivationCode returns a short human-typed code and its stored hash.
func newActivationCode(callsign string) (string, string, error) {
	limit := big.NewInt(int64(len(activationAlphabet)))
	var b strings.Builder
	for i := 0; i < activationCodeLength; i++ {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", "", fmt.Errorf("activation code entropy: %w", err)
		}
		b.WriteByte(activationAlphabet[n.Int64()])
	}
	code := b.String()
	return code, activationHash(callsign, code), nil
}

// activationHash binds a code to its callsign, so a code cannot be replayed
// against a different account even if two happened to collide.
func activationHash(callsign, code string) string {
	return hashToken(strings.ToUpper(callsign) + "|" + normalizeActivationCode(code))
}

func normalizeActivationCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")
	return code
}

// issueActivationCode replaces any pending code for callsign and returns the
// plaintext code to email.
func (s *server) issueActivationCode(callsign string, at time.Time) (string, error) {
	code, hash, err := newActivationCode(callsign)
	if err != nil {
		return "", err
	}
	row := dbActivation{
		Callsign:  normalizeCallsign(callsign),
		CodeHash:  hash,
		ExpiresAt: at.Add(activationCodeTTL).Unix(),
		CreatedAt: at.Unix(),
	}
	if err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "callsign"}}, UpdateAll: true}).Create(&row).Error; err != nil {
		return "", err
	}
	return code, nil
}

// verifyActivationCode consumes one attempt against the pending code.
func (s *server) verifyActivationCode(callsign, code string, at time.Time) error {
	key := normalizeCallsign(callsign)
	row := dbActivation{}
	err := s.db.Where("callsign = ?", key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errActivationExpired
	}
	if err != nil {
		return err
	}
	if at.Unix() > row.ExpiresAt {
		_ = s.clearActivationCode(key)
		return errActivationExpired
	}
	if row.Attempts >= activationAttempts {
		_ = s.clearActivationCode(key)
		return errActivationTooMany
	}
	if activationHash(key, code) != row.CodeHash {
		// Count the failure before returning, so a brute-force loop burns its
		// attempts even if the caller ignores the error.
		if err := s.db.Model(&dbActivation{}).Where("callsign = ?", key).
			UpdateColumn("attempts", gorm.Expr("attempts + 1")).Error; err != nil {
			return err
		}
		return errActivationWrong
	}
	return s.clearActivationCode(key)
}

func (s *server) clearActivationCode(callsign string) error {
	return s.db.Delete(&dbActivation{}, "callsign = ?", normalizeCallsign(callsign)).Error
}

// issueToken stores a link token and returns the plaintext to put in the URL.
func (s *server) issueToken(purpose, callsign, email, changeID string, ttl time.Duration, at time.Time) (string, error) {
	token, hash, err := newToken()
	if err != nil {
		return "", err
	}
	row := dbToken{
		TokenHash: hash,
		Purpose:   purpose,
		Callsign:  normalizeCallsign(callsign),
		Email:     sanitizeSingleLineText(email),
		ChangeID:  changeID,
		CreatedAt: at.Unix(),
		ExpiresAt: at.Add(ttl).Unix(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return "", err
	}
	return token, nil
}

// recentTokens counts tokens of a purpose issued for callsign since `since`.
func (s *server) recentTokens(purpose, callsign string, since time.Time) (int64, error) {
	var count int64
	err := s.db.Model(&dbToken{}).
		Where("purpose = ? AND callsign = ? AND created_at >= ?", purpose, normalizeCallsign(callsign), since.Unix()).
		Count(&count).Error
	return count, err
}

// claimToken resolves a presented token, checks it is live, and marks it
// consumed in one transaction. Returning the row is what lets the caller know
// which account it belongs to.
func (s *server) claimToken(raw, purpose string, at time.Time) (dbToken, error) {
	var claimed dbToken
	err := s.db.Transaction(func(tx *gorm.DB) error {
		row := dbToken{}
		if err := tx.Where("token_hash = ? AND purpose = ?", hashToken(strings.TrimSpace(raw)), purpose).Take(&row).Error; err != nil {
			return errTokenInvalid
		}
		if row.ConsumedAt != 0 || at.Unix() > row.ExpiresAt {
			return errTokenInvalid
		}
		if err := tx.Model(&dbToken{}).Where("id = ?", row.ID).UpdateColumn("consumed_at", at.Unix()).Error; err != nil {
			return err
		}
		claimed = row
		return nil
	})
	return claimed, err
}

// peekToken reports what a token is for without consuming it, so a GET can show
// the right form and a confirmation page without spending the link.
func (s *server) peekToken(raw string) (dbToken, bool) {
	row := dbToken{}
	err := s.db.Where("token_hash = ?", hashToken(strings.TrimSpace(raw))).Take(&row).Error
	if err != nil {
		return dbToken{}, false
	}
	if row.ConsumedAt != 0 || time.Now().Unix() > row.ExpiresAt {
		return row, false
	}
	return row, true
}

// emailChange is one pending double-confirmation of an email address.
type emailChange struct {
	ChangeID string
	Callsign string
	OldEmail string
	NewEmail string
	OldDone  bool
	NewDone  bool
}

// emailChangeState reports the confirmation state of one change, counting a
// half as done once its token has been consumed. It deliberately looks at
// consumed rows too: the request that consumes the second half must still be
// able to see that the first half happened, or the change would never apply.
func (s *server) emailChangeState(changeID string) (emailChange, bool, error) {
	rows := []dbToken{}
	err := s.db.Where("change_id = ? AND purpose IN ?", changeID,
		[]string{tokenPurposeEmailOld, tokenPurposeEmailNew}).Order("id").Find(&rows).Error
	if err != nil {
		return emailChange{}, false, err
	}
	if len(rows) == 0 {
		return emailChange{}, false, nil
	}
	change := emailChange{ChangeID: changeID, Callsign: rows[0].Callsign, NewEmail: rows[0].Email}
	for _, row := range rows {
		switch row.Purpose {
		case tokenPurposeEmailOld:
			change.OldDone = row.ConsumedAt != 0
		case tokenPurposeEmailNew:
			change.NewDone = row.ConsumedAt != 0
		}
	}
	return change, true, nil
}

// pendingEmailChange reports the change a callsign still has to finish, which is
// what the profile page shows. A change whose tokens have both been consumed is
// finished, so it is not reported here.
func (s *server) pendingEmailChange(callsign string) (emailChange, bool, error) {
	rows := []dbToken{}
	err := s.db.Where("callsign = ? AND purpose IN ? AND consumed_at = 0",
		normalizeCallsign(callsign), []string{tokenPurposeEmailOld, tokenPurposeEmailNew}).
		Order("id").Find(&rows).Error
	if err != nil {
		return emailChange{}, false, err
	}
	if len(rows) == 0 {
		return emailChange{}, false, nil
	}
	change := emailChange{ChangeID: rows[0].ChangeID, Callsign: rows[0].Callsign, NewEmail: rows[0].Email}
	for _, row := range rows {
		switch row.Purpose {
		case tokenPurposeEmailOld:
			change.OldDone = true
		case tokenPurposeEmailNew:
			change.NewDone = true
		}
	}
	return change, true, nil
}

func newChangeID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		// A collision here would only group two unrelated changes; fall back to
		// a timestamp rather than failing the request.
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}

// beginEmailChange starts the double confirmation and reports what was sent.
//
// Two tokens share one change ID: one goes to the current address (approve) and
// one to the new address (validate). Both must be consumed before the address
// changes, which stops a stolen session from silently redirecting mail.
func (s *server) beginEmailChange(callsign, oldEmail, newEmail, lang string) error {
	oldEmail = strings.TrimSpace(oldEmail)
	newEmail = strings.TrimSpace(newEmail)
	if oldEmail == newEmail {
		return nil
	}
	changeID := newChangeID()

	oldToken, err := s.issueToken(tokenPurposeEmailOld, callsign, newEmail, changeID, emailChangeTTL, time.Now())
	if err != nil {
		return err
	}
	newToken, err := s.issueToken(tokenPurposeEmailNew, callsign, newEmail, changeID, emailChangeTTL, time.Now())
	if err != nil {
		return err
	}

	confirm := emailChangeMail(lang, callsign, s.mail.WebURL("/email/confirm?token="+oldToken), s.cfg.name, newEmail, true)
	confirm.To = oldEmail
	if err := s.mail.Send(confirm); err != nil {
		return fmt.Errorf("confirm mail to current address: %w", err)
	}
	validate := emailChangeMail(lang, callsign, s.mail.WebURL("/email/confirm?token="+newToken), s.cfg.name, newEmail, false)
	validate.To = newEmail
	if err := s.mail.Send(validate); err != nil {
		return fmt.Errorf("validation mail to new address: %w", err)
	}
	s.logBBSAction(callsign, "email_change_requested", "from=%q to=%q expires_in=%s", oldEmail, newEmail, emailChangeTTL)
	return nil
}

// completeEmailChange applies the address once both halves have been confirmed.
// It is called after a token has been consumed, so a half-confirmed change waits
// for the other link rather than changing anything now.
func (s *server) completeEmailChange(change emailChange) (bool, error) {
	if !change.OldDone || !change.NewDone {
		return false, nil
	}
	err := s.db.Model(&dbUser{}).Where("callsign = ?", change.Callsign).
		Update("email", sanitizeSingleLineText(change.NewEmail)).Error
	if err != nil {
		return false, err
	}
	s.db.Model(&dbToken{}).Where("change_id = ? AND consumed_at = 0", change.ChangeID).
		UpdateColumn("consumed_at", time.Now().Unix())
	s.logBBSAction(change.Callsign, "email_change_applied", "email=%q", change.NewEmail)
	return true, nil
}

// sendPasswordReset issues and sends a reset link when the identifier matches an
// active account. Unknown identifiers, disabled accounts and over-quota requests
// are silent no-ops: the caller always sees the same message.
func (s *server) sendPasswordReset(identifier string) error {
	profile, callsign, found, err := s.lookupAccount(identifier)
	if err != nil {
		return err
	}
	if !found || profile.Disabled || strings.TrimSpace(profile.Email) == "" {
		return nil
	}
	count, err := s.recentTokens(tokenPurposePasswordReset, callsign, time.Now().Add(-time.Hour))
	if err != nil {
		return err
	}
	if count >= resetRequestsPerHour {
		return fmt.Errorf("reset quota reached for %s", callsign)
	}
	token, err := s.issueToken(tokenPurposePasswordReset, callsign, profile.Email, "", passwordResetTTL, time.Now())
	if err != nil {
		return err
	}
	lang := profile.Language
	if lang == "" {
		lang = "en"
	}
	msg := passwordResetMail(lang, callsign, s.mail.WebURL("/reset?token="+token), s.cfg.name)
	msg.To = profile.Email
	if err := s.mail.Send(msg); err != nil {
		return err
	}
	s.logBBSAction(callsign, "password_reset_mail_sent", "to=%q expires_in=%s", profile.Email, passwordResetTTL)
	return nil
}

// lookupAccount resolves a callsign or an email address to an account.
func (s *server) lookupAccount(identifier string) (dbUser, string, bool, error) {
	identifier = sanitizeSingleLineText(identifier)
	if identifier == "" {
		return dbUser{}, "", false, nil
	}
	var byCallsign dbUser
	err := s.db.Where("callsign = ?", normalizeCallsign(identifier)).Limit(1).Find(&byCallsign).Error
	if err != nil {
		return dbUser{}, "", false, err
	}
	if byCallsign.Callsign != "" {
		return byCallsign, byCallsign.Callsign, true, nil
	}
	if !strings.Contains(identifier, "@") {
		return dbUser{}, "", false, nil
	}
	var users []dbUser
	if err := s.db.Find(&users).Error; err != nil {
		return dbUser{}, "", false, err
	}
	want := strings.ToLower(identifier)
	for _, user := range users {
		if strings.ToLower(strings.TrimSpace(user.Email)) == want {
			return user, user.Callsign, true, nil
		}
	}
	return dbUser{}, "", false, nil
}

// registrationAllowed reports whether address may create another account now.
//
// It does not consume a slot: a caller that opens the registration form and
// cancels must not be charged. The count is incremented once an account actually
// exists, so the check-then-record gap can be crossed twice by simultaneous
// registrations from one address, which overshoots the limit by at most the
// number of concurrent requests rather than by an unbounded amount.
func (s *server) registrationAllowed(address string, at time.Time) (bool, error) {
	row := dbRegistration{}
	err := s.db.Where("address = ?", address).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if at.Sub(time.Unix(row.WindowStart, 0)) > registrationWindow {
		return true, nil
	}
	return row.Count < registrationLimit, nil
}

// recordRegistration counts one new account against address, starting a fresh
// window when the previous one has expired.
func (s *server) recordRegistration(address string, at time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		row := dbRegistration{}
		err := tx.Where("address = ?", address).Take(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row = dbRegistration{Address: address, WindowStart: at.Unix()}
		}
		if row.WindowStart == 0 || at.Sub(time.Unix(row.WindowStart, 0)) > registrationWindow {
			row.Count = 0
			row.WindowStart = at.Unix()
		}
		row.Count++
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "address"}}, UpdateAll: true}).Create(&row).Error
	})
}

// pruneAccountTables drops counter and token rows that can no longer do
// anything, so none of these tables grows without bound. The SSH BBS runs the
// same cleanup on its nightly timer.
func (s *server) pruneAccountTables(at time.Time) error {
	if err := s.db.Where("window_start < ?", at.Add(-registrationRetention).Unix()).
		Delete(&dbRegistration{}).Error; err != nil {
		return err
	}
	cutoff := at.Add(-tokenRetention).Unix()
	if err := s.db.Where("expires_at < ? AND created_at < ?", cutoff, cutoff).Delete(&dbToken{}).Error; err != nil {
		return err
	}
	if err := s.db.Where("expires_at < ?", at.Unix()).Delete(&dbActivation{}).Error; err != nil {
		return err
	}
	// In-memory login-throttle rows are pruned on insert; this is the rest.
	s.pruneAuthThrottle(at)
	return nil
}

// startMaintenance runs the account-table cleanup once at startup and then on an
// interval. The SSH BBS does the same work on its nightly timer; this app is
// long-running and has no session lifecycle to hang it on, so it owns a timer.
func (s *server) startMaintenance(every time.Duration) {
	if every <= 0 {
		return
	}
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		if err := s.pruneAccountTables(time.Now()); err != nil {
			s.logBBSAction("system", "maintenance_failed", "error=%q", err.Error())
		}
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.pruneAccountTables(time.Now()); err != nil {
				s.logBBSAction("system", "maintenance_failed", "error=%q", err.Error())
			}
		}
	}()
}
