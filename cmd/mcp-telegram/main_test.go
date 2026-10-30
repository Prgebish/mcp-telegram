package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"rsc.io/qr"
)

func TestDescribeSentCode(t *testing.T) {
	tests := []struct {
		name string
		typ  tg.AuthSentCodeTypeClass
		want string
	}{
		{"app", &tg.AuthSentCodeTypeApp{Length: 5}, `"Telegram" service chat`},
		{"sms", &tg.AuthSentCodeTypeSMS{Length: 5}, "SMS (5 digits)"},
		{"call", &tg.AuthSentCodeTypeCall{Length: 5}, "phone call"},
		{"missed call", &tg.AuthSentCodeTypeMissedCall{Prefix: "+7900", Length: 4}, "last 4 digits of the number starting with +7900"},
		{"email", &tg.AuthSentCodeTypeEmailCode{EmailPattern: "a***@example.com", Length: 6}, "email a***@example.com"},
		{"fragment", &tg.AuthSentCodeTypeFragmentSMS{URL: "https://fragment.com/x", Length: 5}, "Fragment: https://fragment.com/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := describeSentCode(&tg.AuthSentCode{Type: tt.typ})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("got %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestDescribeSentCode_SetUpEmailRequired(t *testing.T) {
	_, err := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeSetUpEmailRequired{}})
	if err == nil {
		t.Fatal("expected error for SetUpEmailRequired")
	}
}

type invokerFunc func(ctx context.Context, input bin.Encoder, output bin.Decoder) error

func (f invokerFunc) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	return f(ctx, input, output)
}

func testLogin(api *tg.Client, input string, loggedIn qrlogin.LoggedIn) login {
	return login{
		auth:     auth.NewClient(api, rand.Reader, 1, "hash"),
		qr:       qrlogin.NewQR(api, 1, "hash", qrlogin.Options{}),
		loggedIn: loggedIn,
		reader:   bufio.NewReader(strings.NewReader(input)),
		out:      io.Discard,
	}
}

func TestStart_ChoosesMethod(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantQR     bool
		wantSignIn bool
		wantErr    bool
	}{
		{name: "enter defaults to QR", input: "\n", wantQR: true},
		{name: "QR", input: "1\n", wantQR: true},
		{name: "unknown choice asks again", input: "+10000000000\n1\n", wantQR: true},
		{name: "phone", input: "2\n+10000000000\n12345\n", wantSignIn: true},
		{name: "phone required", input: "2\n\n", wantErr: true},
		{name: "closed stdin starts nothing", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loggedIn := make(chan struct{}, 1)
			var exports int
			var signedIn bool
			api := tg.NewClient(invokerFunc(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
				switch input.(type) {
				case *tg.AuthExportLoginTokenRequest:
					exports++
					box := output.(*tg.AuthLoginTokenBox)
					if exports == 1 {
						box.LoginToken = &tg.AuthLoginToken{
							Token:   []byte("token"),
							Expires: int(time.Now().Add(time.Minute).Unix()),
						}
						loggedIn <- struct{}{} // The user scanned the code.
						return nil
					}
					box.LoginToken = &tg.AuthLoginTokenSuccess{
						Authorization: &tg.AuthAuthorization{User: &tg.User{}},
					}
				case *tg.AuthSendCodeRequest:
					output.(*tg.AuthSentCodeBox).SentCode = &tg.AuthSentCode{
						Type:          &tg.AuthSentCodeTypeApp{Length: 5},
						PhoneCodeHash: "h1",
					}
				case *tg.AuthSignInRequest:
					signedIn = true
					output.(*tg.AuthAuthorizationBox).Authorization = &tg.AuthAuthorization{User: &tg.User{}}
				default:
					t.Fatalf("unexpected request %T", input)
				}
				return nil
			}))
			l := testLogin(api, tt.input, loggedIn)

			err := l.start(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("start: err = %v, wantErr %v", err, tt.wantErr)
			}
			if got := exports > 0; got != tt.wantQR {
				t.Errorf("QR login started = %v, want %v", got, tt.wantQR)
			}
			if signedIn != tt.wantSignIn {
				t.Errorf("signed in by code = %v, want %v", signedIn, tt.wantSignIn)
			}
		})
	}
}

func TestStart_StopsWhenCancelled(t *testing.T) {
	api := tg.NewClient(invokerFunc(func(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
		t.Fatalf("unexpected request %T after cancel", input)
		return nil
	}))
	// Ctrl-C followed by Enter: the empty line must not start QR login.
	l := testLogin(api, "\n", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := l.start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSignIn_ResendUsesNewCodeHash(t *testing.T) {
	var resendHash, signInHashes []string
	api := tg.NewClient(invokerFunc(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		switch req := input.(type) {
		case *tg.AuthSendCodeRequest:
			output.(*tg.AuthSentCodeBox).SentCode = &tg.AuthSentCode{
				Type:          &tg.AuthSentCodeTypeApp{Length: 5},
				PhoneCodeHash: "h1",
				NextType:      &tg.AuthCodeTypeSMS{},
			}
		case *tg.AuthResendCodeRequest:
			resendHash = append(resendHash, req.PhoneCodeHash)
			output.(*tg.AuthSentCodeBox).SentCode = &tg.AuthSentCode{
				Type:          &tg.AuthSentCodeTypeSMS{Length: 5},
				PhoneCodeHash: "h2",
			}
		case *tg.AuthSignInRequest:
			signInHashes = append(signInHashes, req.PhoneCodeHash)
			if req.PhoneCode != "12345" {
				return tgerr.New(400, "PHONE_CODE_INVALID")
			}
			output.(*tg.AuthAuthorizationBox).Authorization = &tg.AuthAuthorization{User: &tg.User{}}
		default:
			t.Fatalf("unexpected request %T", input)
		}
		return nil
	}))
	l := testLogin(api, "resend\n00000\n12345\n", nil)

	if err := l.withCode(context.Background(), "+10000000000"); err != nil {
		t.Fatalf("signIn: %v", err)
	}
	if len(resendHash) != 1 || resendHash[0] != "h1" {
		t.Errorf("resendCode hashes = %v, want [h1]", resendHash)
	}
	if want := []string{"h2", "h2"}; strings.Join(signInHashes, ",") != strings.Join(want, ",") {
		t.Errorf("signIn hashes = %v, want %v", signInHashes, want)
	}
}

func TestSignIn_ResendUnavailableKeepsCode(t *testing.T) {
	var signInHash string
	api := tg.NewClient(invokerFunc(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		switch req := input.(type) {
		case *tg.AuthSendCodeRequest:
			output.(*tg.AuthSentCodeBox).SentCode = &tg.AuthSentCode{
				Type:          &tg.AuthSentCodeTypeApp{Length: 5},
				PhoneCodeHash: "h1",
			}
		case *tg.AuthResendCodeRequest:
			return tgerr.New(400, "SEND_CODE_UNAVAILABLE")
		case *tg.AuthSignInRequest:
			signInHash = req.PhoneCodeHash
			output.(*tg.AuthAuthorizationBox).Authorization = &tg.AuthAuthorization{User: &tg.User{}}
		}
		return nil
	}))
	l := testLogin(api, "resend\n12345\n", nil)

	if err := l.withCode(context.Background(), "+10000000000"); err != nil {
		t.Fatalf("signIn: %v", err)
	}
	if signInHash != "h1" {
		t.Errorf("signIn hash = %q, want h1", signInHash)
	}
}

func TestSignIn_SwitchToQR(t *testing.T) {
	loggedIn := make(chan struct{}, 1)
	var exports int
	api := tg.NewClient(invokerFunc(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		switch input.(type) {
		case *tg.AuthSendCodeRequest:
			output.(*tg.AuthSentCodeBox).SentCode = &tg.AuthSentCode{
				Type:          &tg.AuthSentCodeTypeApp{Length: 5},
				PhoneCodeHash: "h1",
			}
		case *tg.AuthExportLoginTokenRequest:
			exports++
			box := output.(*tg.AuthLoginTokenBox)
			if exports == 1 {
				box.LoginToken = &tg.AuthLoginToken{
					Token:   []byte("token"),
					Expires: int(time.Now().Add(time.Minute).Unix()),
				}
				loggedIn <- struct{}{} // The user scanned the code.
				return nil
			}
			box.LoginToken = &tg.AuthLoginTokenSuccess{
				Authorization: &tg.AuthAuthorization{User: &tg.User{}},
			}
		case *tg.AuthSignInRequest:
			t.Fatal("sign in by code must not be called")
		default:
			t.Fatalf("unexpected request %T", input)
		}
		return nil
	}))
	l := testLogin(api, "qr\n", loggedIn)

	if err := l.withCode(context.Background(), "+10000000000"); err != nil {
		t.Fatalf("withCode: %v", err)
	}
	if exports != 2 {
		t.Errorf("exportLoginToken calls = %d, want 2 (export, then import)", exports)
	}
}

func TestSignIn_QRNeedsPassword(t *testing.T) {
	loggedIn := make(chan struct{}, 1)
	loggedIn <- struct{}{}
	api := tg.NewClient(invokerFunc(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		switch input.(type) {
		case *tg.AuthExportLoginTokenRequest:
			return tgerr.New(401, "SESSION_PASSWORD_NEEDED")
		case *tg.AccountGetPasswordRequest:
			t.Fatal("password must not be requested before it is entered")
		}
		return nil
	}))
	l := testLogin(api, "", loggedIn)

	// Stdin is not a terminal in tests, so reading the password fails: this
	// shows withQR went on to the 2FA prompt instead of returning the RPC error.
	err := l.withQR(context.Background())
	if err == nil || !strings.Contains(err.Error(), "read password") {
		t.Fatalf("err = %v, want a 2FA password prompt", err)
	}
}

func TestRenderQR(t *testing.T) {
	code, err := qr.Encode("tg://login?token=abc", qr.M)
	if err != nil {
		t.Fatal(err)
	}
	out := renderQR(code)
	n := code.Size + 8 // quiet zone of 4 on each side
	if got := strings.Count(out, "\n"); got != n {
		t.Errorf("lines = %d, want %d", got, n)
	}
	white, black := strings.Count(out, "\x1b[48;5;231m  "), strings.Count(out, "\x1b[48;5;16m  ")
	if white+black != n*n {
		t.Errorf("modules = %d, want %d", white+black, n*n)
	}
	dark := 0
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				dark++
			}
		}
	}
	if black != dark {
		t.Errorf("black modules = %d, want %d", black, dark)
	}
	// The first row is the quiet zone: all white.
	if first := out[:strings.Index(out, "\n")]; strings.Contains(first, "48;5;16m") {
		t.Error("quiet zone row contains black modules")
	}
}

// testPasswordAlgo is the 2FA KDF from gotd's own auth tests.
func testPasswordAlgo(t *testing.T) *tg.PasswordKdfAlgoSHA256SHA256PBKDF2HMACSHA512iter100000SHA256ModPow {
	t.Helper()
	unhex := func(s string) []byte {
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	return &tg.PasswordKdfAlgoSHA256SHA256PBKDF2HMACSHA512iter100000SHA256ModPow{
		Salt1: unhex("4D11FB6BEC38F9D2546BB0F61E4F1C99A1BC0DB8F0D5F35B1291B37B213123D7ED48F3C6794D495B"),
		Salt2: unhex("A1B181AAFE88188680AE32860D60BB01"),
		G:     3,
		P: unhex("C71CAEB9C6B1C9048E6C522F70F13F73980D40238E3E21C14934D037563D930F" +
			"48198A0AA7C14058229493D22530F4DBFA336F6E0AC925139543AED44CCE7C37" +
			"20FD51F69458705AC68CD4FE6B6B13ABDC9746512969328454F18FAF8C595F64" +
			"2477FE96BB2A941D5BCD1D4AC8CC49880708FA9B378E3C4F3A9060BEE67CF9A4" +
			"A4A695811051907E162753B56B0F6B410DBA74D8A84B2A14B3144E0EF1284754" +
			"FD17ED950D5965B4B9DD46582DB1178D169C6BC465B0D6FF9CA3928FEF5B9AE4" +
			"E418FC15E83EBEA0F87FA9FF5EED70050DED2849F47BF959D956850CE929851F" +
			"0D8115F635B105EE2E4E15D04B2454BF6F4FADF034B10403119CD8E3B92FCC5B"),
	}
}

func TestEnterPassword_RetriesInvalid(t *testing.T) {
	algo := testPasswordAlgo(t)
	checks := 0
	api := tg.NewClient(invokerFunc(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		switch input.(type) {
		case *tg.AccountGetPasswordRequest:
			pwd := &tg.AccountPassword{
				NewAlgo:       algo,
				NewSecureAlgo: &tg.SecurePasswordKdfAlgoPBKDF2HMACSHA512iter100000{},
			}
			pwd.SetCurrentAlgo(algo)
			*output.(*tg.AccountPassword) = *pwd
		case *tg.AuthCheckPasswordRequest:
			checks++
			if checks == 1 {
				return tgerr.New(400, "PASSWORD_HASH_INVALID")
			}
			output.(*tg.AuthAuthorizationBox).Authorization = &tg.AuthAuthorization{User: &tg.User{}}
		default:
			t.Fatalf("unexpected request %T", input)
		}
		return nil
	}))
	l := testLogin(api, "", nil)
	passwords := []string{"wrong", "right"}
	l.readPassword = func() ([]byte, error) {
		p := passwords[0]
		passwords = passwords[1:]
		return []byte(p), nil
	}

	if err := l.enterPassword(context.Background()); err != nil {
		t.Fatalf("enterPassword: %v", err)
	}
	if checks != 2 {
		t.Errorf("checkPassword calls = %d, want 2 (wrong, then right)", checks)
	}
}

func TestEnterCode_StopsWhenCancelled(t *testing.T) {
	api := tg.NewClient(invokerFunc(func(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
		t.Fatalf("unexpected request %T after cancel", input)
		return nil
	}))
	// Ctrl-C followed by Enter: the empty line must not bring the prompt back.
	l := testLogin(api, "\n\n", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := l.enterCode(ctx, "+10000000000", "h1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSanitizeTerminal(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Dev Team 🚀", "Dev Team 🚀"},
		{"evil\x1b]0;pwned\x07title", "evil\uFFFD]0;pwned\uFFFDtitle"},
		{"line\nbreak", "line\uFFFDbreak"},
		{"abc\u202Edcba", "abc\uFFFDdcba"},
	}
	for _, tt := range tests {
		if got := sanitizeTerminal(tt.in); got != tt.want {
			t.Errorf("sanitizeTerminal(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
