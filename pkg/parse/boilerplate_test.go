package parse

import (
	"strings"
	"testing"
)

// TestParseEmailBody_WebexBoilerplate is a REPRODUCTION TEST for bug pf-0017b0.
//
// BUG: ParseEmailBody() performs HTML-to-text conversion and quoted reply separation,
// but has zero concept of conferencing boilerplate (Webex, Teams, Zoom), signatures,
// or disclaimers. Webex meeting invitation blocks appended to emails are passed through
// verbatim to the CleanBody and NewContent fields.
//
// CONSEQUENCE: Short emails with Webex meeting blocks produce embeddings dominated by
// conferencing boilerplate rather than the actual email content. A 2-sentence email
// followed by a 10-line Webex block will have its embedding skewed toward Webex
// infrastructure text, not the human-authored content.
//
// EXPECTED BEHAVIOR: CleanBody (and NewContent) should NOT contain Webex boilerplate
// markers. The boilerplate should be stripped or separated before the content reaches
// the embedding stage.
//
// This test FAILS against current code because no boilerplate stripping exists.
func TestParseEmailBody_WebexBoilerplate(t *testing.T) {
	// Realistic email: short 2-sentence message + full Webex conferencing block.
	// This is the exact format sent by Cisco Webex when users schedule meetings via email.
	input := `Hi team, the CLIC review is scheduled for Thursday at 2pm.

Please confirm your attendance.

-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-
- Do not delete or change any of the following text. -
Join my Webex Personal Room meeting.
More ways to join:
Join from the meeting link
https://company.webex.com/meet/jsmith

Meeting number (access code): 920 443 465
Meeting password: abc123

Join by phone
1-877-668-4490 Call-in toll-free number (US/Canada)
1-408-792-6300 Call-in toll number (US/Canada)
Global call-in numbers | Toll-free calling restrictions

Join from a video system or application
Dial sip:920443465@company.webex.com

(c) 2021 Cisco Systems, Inc. and/or its affiliates. All rights reserved.`

	result := ParseEmailBody(input, "")

	// The actual email content should be present in the output.
	if !strings.Contains(result.CleanBody, "CLIC review is scheduled for Thursday") {
		t.Errorf("CleanBody should contain the actual email content.\nGot: %s", result.CleanBody)
	}
	if !strings.Contains(result.CleanBody, "confirm your attendance") {
		t.Errorf("CleanBody should contain the actual email content.\nGot: %s", result.CleanBody)
	}

	// FAILING ASSERTIONS: These assert that boilerplate is NOT present.
	// All will FAIL until boilerplate stripping is implemented.

	// The Webex separator line should be stripped.
	if strings.Contains(result.CleanBody, "-~-~-~-~") {
		t.Errorf("CleanBody should NOT contain Webex separator line '-~-~-~-~'.\nGot: %s", result.CleanBody)
	}

	// "Join my Webex Personal Room meeting" is a signature boilerplate phrase.
	if strings.Contains(result.CleanBody, "Join my Webex Personal Room meeting") {
		t.Errorf("CleanBody should NOT contain Webex boilerplate 'Join my Webex Personal Room meeting'.\nGot: %s", result.CleanBody)
	}

	// Meeting access code line is pure infrastructure noise.
	if strings.Contains(result.CleanBody, "Meeting number (access code)") {
		t.Errorf("CleanBody should NOT contain Webex meeting number line.\nGot: %s", result.CleanBody)
	}

	// Copyright footer from Cisco should be stripped.
	if strings.Contains(result.CleanBody, "(c) 2021 Cisco Systems") {
		t.Errorf("CleanBody should NOT contain Cisco copyright footer.\nGot: %s", result.CleanBody)
	}

	// Dial-in phone numbers are infrastructure noise, not email content.
	if strings.Contains(result.CleanBody, "1-877-668-4490") {
		t.Errorf("CleanBody should NOT contain dial-in phone number.\nGot: %s", result.CleanBody)
	}

	// NewContent should also be clean — it is what reaches the embedding stage.
	if strings.Contains(result.NewContent, "-~-~-~-~") {
		t.Errorf("NewContent should NOT contain Webex separator line.\nGot: %s", result.NewContent)
	}
	if strings.Contains(result.NewContent, "Join my Webex Personal Room meeting") {
		t.Errorf("NewContent should NOT contain Webex boilerplate.\nGot: %s", result.NewContent)
	}
	if strings.Contains(result.NewContent, "Meeting number (access code)") {
		t.Errorf("NewContent should NOT contain Webex meeting number line.\nGot: %s", result.NewContent)
	}
}

// TestParseEmailBody_WebexBoilerplate_HTMLFormat tests the same bug with HTML email input.
// Webex meeting invitations are commonly appended to HTML emails. The boilerplate block
// should be stripped even when the email body is delivered as HTML.
//
// This test FAILS against current code because no boilerplate stripping exists.
func TestParseEmailBody_WebexBoilerplate_HTMLFormat(t *testing.T) {
	// HTML version of a short reply with Webex conferencing block appended.
	htmlInput := `<html><body>
<div>Hi team, the CLIC review is scheduled for Thursday at 2pm.</div>
<div>Please confirm your attendance.</div>
<div>-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-</div>
<div>- Do not delete or change any of the following text. -</div>
<div>Join my Webex Personal Room meeting.</div>
<div>Meeting number (access code): 920 443 465</div>
<div>Join by phone</div>
<div>1-877-668-4490 Call-in toll-free number (US/Canada)</div>
<div>(c) 2021 Cisco Systems, Inc. and/or its affiliates. All rights reserved.</div>
</body></html>`

	result := ParseEmailBody("", htmlInput)

	// The actual email content should be present.
	if !strings.Contains(result.CleanBody, "CLIC review is scheduled for Thursday") {
		t.Errorf("CleanBody should contain actual email content.\nGot: %s", result.CleanBody)
	}

	// FAILING ASSERTIONS: boilerplate should be stripped from HTML emails too.
	if strings.Contains(result.CleanBody, "-~-~-~-~") {
		t.Errorf("CleanBody should NOT contain Webex separator line.\nGot: %s", result.CleanBody)
	}
	if strings.Contains(result.CleanBody, "Join my Webex Personal Room meeting") {
		t.Errorf("CleanBody should NOT contain Webex boilerplate.\nGot: %s", result.CleanBody)
	}
	if strings.Contains(result.CleanBody, "Meeting number (access code)") {
		t.Errorf("CleanBody should NOT contain Webex meeting number line.\nGot: %s", result.CleanBody)
	}
	if strings.Contains(result.CleanBody, "(c) 2021 Cisco Systems") {
		t.Errorf("CleanBody should NOT contain Cisco copyright footer.\nGot: %s", result.CleanBody)
	}
	if strings.Contains(result.CleanBody, "1-877-668-4490") {
		t.Errorf("CleanBody should NOT contain dial-in phone number.\nGot: %s", result.CleanBody)
	}
}

// TestParseEmailBody_WebexBoilerplate_ShortEmailDomination tests the specific
// embedding-skew scenario: a short 2-sentence email dominated by a Webex block.
//
// The boilerplate word count exceeds the actual email word count 5:1.
// Before stripping, NewContent contains ~90 tokens of Webex noise.
// After stripping, NewContent should be <= 20 tokens of actual content.
//
// This test FAILS against current code because no boilerplate stripping exists.
func TestParseEmailBody_WebexBoilerplate_ShortEmailDomination(t *testing.T) {
	// Minimal 2-sentence email with a full Webex block.
	input := `Approved. Let's proceed.

-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-~-
- Do not delete or change any of the following text. -
Join my Webex Personal Room meeting.
More ways to join:
Join from the meeting link
https://company.webex.com/meet/jsmith

Meeting number (access code): 920 443 465
Meeting password: abc123

Join by phone
1-877-668-4490 Call-in toll-free number (US/Canada)
1-408-792-6300 Call-in toll number (US/Canada)
Global call-in numbers | Toll-free calling restrictions

Join from a video system or application
Dial sip:920443465@company.webex.com

(c) 2021 Cisco Systems, Inc. and/or its affiliates. All rights reserved.`

	result := ParseEmailBody(input, "")

	// Count words in NewContent — should be just the 2-sentence reply, not the Webex block.
	words := strings.Fields(result.NewContent)
	const maxAcceptableWords = 20
	if len(words) > maxAcceptableWords {
		t.Errorf(
			"NewContent has %d words but should have <= %d after boilerplate stripping.\n"+
				"Boilerplate is dominating the content field that feeds embeddings.\n"+
				"NewContent was:\n%s",
			len(words), maxAcceptableWords, result.NewContent,
		)
	}

	// The actual 2-sentence content must still be present.
	if !strings.Contains(result.NewContent, "Approved") {
		t.Errorf("NewContent should contain the actual reply 'Approved'.\nGot: %s", result.NewContent)
	}
	if !strings.Contains(result.NewContent, "Let's proceed") {
		t.Errorf("NewContent should contain the actual reply 'Let's proceed'.\nGot: %s", result.NewContent)
	}
}
