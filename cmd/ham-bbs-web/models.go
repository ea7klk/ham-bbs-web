package main

type dbUser struct {
	Callsign     string `gorm:"primaryKey;size:32"`
	FullName     string
	Email        string
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
