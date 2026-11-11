package main

type dbUser struct {
	Callsign string `gorm:"primaryKey;size:32"`
	FullName string
	// Email is indexed with COLLATE NOCASE, identically to the classic SSH BBS.
	// SQLite only uses that index for a query that names the collation, so every
	// lookup by address must say COLLATE NOCASE — see emailLookup.
	Email        string `gorm:"index:idx_users_email_nocase,collate:nocase"`
	Maidenhead   string
	Language     string
	EnableAPRS   bool
	QTH          string
	Rig          string
	PasswordHash string
	IsSysop      bool
	Disabled     bool
	FirstSeen    string
	LastSeen     string
	// Registered is when the account was created. It is written once and never
	// changes, unlike FirstSeen/LastSeen which track presence.
	Registered string
	// Activated is false until the caller proves they can read the address they
	// registered with. Accounts that predate activation are marked true by the
	// backfill in database.go.
	Activated bool
}

type dbBulletin struct {
	ID       uint `gorm:"primaryKey"`
	Position int  `gorm:"index"`
	Title    string
	Body     string
	Updated  string
	From     string
}

type dbBoard struct {
	ID          string `gorm:"primaryKey;size:64"`
	Position    int    `gorm:"index"`
	Name        string
	Description string
	Created     string
}

type dbMessage struct {
	ID       uint   `gorm:"primaryKey"`
	BoardID  string `gorm:"index"`
	ParentID *uint  `gorm:"index"`
	Position int    `gorm:"index"`
	From     string
	Subject  string
	Body     string
	Created  string
	Edited   string
}

type dbAPRSSent struct {
	ID           uint   `gorm:"primaryKey"`
	UserCallsign string `gorm:"index"`
	Position     int    `gorm:"index"`
	At           string
	From         string
	To           string
	Text         string
	Status       string
	Acked        bool
	Passcode     int
	Parts        []dbAPRSSentPart `gorm:"foreignKey:SentID;constraint:OnDelete:CASCADE"`
}

type dbAPRSSentPart struct {
	ID        uint `gorm:"primaryKey"`
	SentID    uint `gorm:"index"`
	Number    int
	Text      string
	Status    string
	Detail    string
	MessageID string `gorm:"index"`
	Acked     bool
}

type dbAPRSReceived struct {
	ID           uint   `gorm:"primaryKey"`
	UserCallsign string `gorm:"index"`
	Position     int    `gorm:"index"`
	At           string
	From         string
	To           string
	Text         string
	Raw          string
}

// The tables below are shared with the classic SSH BBS. It declares them in
// bbs/cmd/bbs/tokens.go and bbs/cmd/bbs/ratelimit.go; the shapes here have to
// match, or a row written by one application cannot be read by the other.

// dbActivation is the pending activation code for one account. Only the hash is
// stored, so a copy of the database cannot be used to activate an account.
type dbActivation struct {
	Callsign  string `gorm:"primaryKey;size:32"`
	CodeHash  string `gorm:"size:64"`
	ExpiresAt int64
	Attempts  int
	CreatedAt int64
}

// dbToken is a redeemable link token: a password reset, or one half of an email
// change. The presented token is hashed and matched against TokenHash.
type dbToken struct {
	ID         uint   `gorm:"primaryKey"`
	TokenHash  string `gorm:"uniqueIndex;size:64"`
	Purpose    string `gorm:"index;size:32"`
	Callsign   string `gorm:"index;size:32"`
	Email      string `gorm:"size:160"`
	ChangeID   string `gorm:"index;size:40"`
	CreatedAt  int64
	ExpiresAt  int64
	ConsumedAt int64
}

// dbAPRSBulletin is one APRS bulletin, shared by all users rather than stored per
// recipient: a bulletin addresses BLNn + a group, not a callsign.
//
// The classic SSH BBS receiver owns the writes. It deduplicates by content, so a
// bulletin retransmitted every few minutes updates one row instead of adding
// copies; the web app only reads this table. The struct has to match
// bbs/cmd/bbs/database.go field for field.
type dbAPRSBulletin struct {
	ID        uint   `gorm:"primaryKey"`
	DedupeKey string `gorm:"uniqueIndex;size:64"`
	FirstSeen string `gorm:"index"`
	LastSeen  string `gorm:"index"`
	SeenCount int
	From      string `gorm:"index"`
	Slot      int
	Group     string `gorm:"size:8;index"`
	Category  string `gorm:"size:24;index"`
	Text      string
	Raw       string
}

// dbRegistration counts accounts created from one client address.
type dbRegistration struct {
	Address     string `gorm:"primaryKey;size:64"`
	Count       int
	WindowStart int64
}

// dbSetting is the key/value store both applications use for migration markers.
type dbSetting struct {
	Key   string `gorm:"primaryKey;size:64"`
	Value string
}

type messageNode struct {
	Row     dbMessage
	Depth   int
	Replies []messageNode
}

type sentAPRS struct {
	ID       uint
	At       string
	From     string
	To       string
	Text     string
	Status   string
	Acked    bool
	Passcode int
	Parts    []sentAPRSPart
}

type sentAPRSPart struct {
	Number    int
	Text      string
	Status    string
	Detail    string
	MessageID string
	Acked     bool
}

type ackBadge struct {
	Icon     string
	Class    string
	LabelKey string
}

type receivedAPRSDetail struct {
	Text string
	Raw  string
}

type aprsTimestamp struct {
	Date string
	Time string
}

type aprsPagination struct {
	Page        int
	PerPage     int
	Total       int64
	Pages       int
	PageSizes   []int
	HasPrevious bool
	HasNext     bool
	PreviousURL string
	NextURL     string
}

type aprsOverviewView struct {
	Sent     aprsPagination
	Received aprsPagination
	// ThursdayGroup names the ANSRVR group, and ThursdayActive says whether the
	// net is running, so the page can explain rather than show a button that will
	// be refused.
	ThursdayGroup  string
	ThursdayActive bool
}

type aprsSentPageView struct {
	Lang       string
	Rows       []dbAPRSSent
	Pagination aprsPagination
}

type aprsReceivedPageView struct {
	Lang       string
	Rows       []dbAPRSReceived
	Pagination aprsPagination
}
