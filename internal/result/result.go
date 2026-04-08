package result

import "time"

type Result struct {
	CheckID              string
	CheckName            string
	CheckGroup           string
	CheckKind            string
	CheckScope           string
	Labels               map[string]string
	Success              bool
	Duration             time.Duration
	HTTPStatusCode       int
	DNSRCode             int
	DNSAnswerCount       int
	ICMPEchoReplies      int
	TLSCertDaysRemaining int
}
