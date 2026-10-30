package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/Prgebish/mcp-telegram/internal/ratelimit"
	tgclient "github.com/Prgebish/mcp-telegram/internal/telegram"
	"github.com/Prgebish/mcp-telegram/internal/tools"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/term"
	"rsc.io/qr"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "auth":
		runAuth()
	case "serve":
		runServe()
	case "dialogs":
		runDialogs()
	case "--config":
		// Backward compat: ./mcp-telegram --config path
		os.Args = append([]string{os.Args[0], "serve"}, os.Args[1:]...)
		runServe()
	case "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  mcp-telegram auth    --config config.yaml   Authenticate with Telegram (QR code or phone code)
  mcp-telegram dialogs --config config.yaml   List all dialogs (type, ref, title) to pick ACL entries
  mcp-telegram serve   --config config.yaml   Start MCP server (default)`)
}

// --- auth command ---

func runAuth() {
	configPath := findFlag("--config", os.Args[2:])
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config flag is required")
		os.Exit(1)
	}

	// Auth only needs telegram credentials, not the full config with ACL.
	tgCfg, err := config.LoadTelegram(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sessionDir := filepath.Dir(tgCfg.SessionPath)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "error creating session directory: %v\n", err)
		os.Exit(1)
	}

	// QR login learns that the code was scanned from updateLoginToken.
	dispatcher := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(dispatcher)

	storage := &session.FileStorage{Path: tgCfg.SessionPath}
	client := telegram.NewClient(tgCfg.AppID, tgCfg.APIHash, telegram.Options{
		SessionStorage: storage,
		Device:         tgclient.Device(),
		UpdateHandler:  dispatcher,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	err = client.Run(ctx, func(ctx context.Context) error {
		// Check if already authenticated — skip login flow if so.
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("check auth status: %w", err)
		}
		if status.Authorized {
			user := status.User
			fmt.Printf("Already authenticated as %s %s (@%s)\n", user.FirstName, user.LastName, user.Username)
			fmt.Printf("Session: %s\n", tgCfg.SessionPath)
			_ = os.Chmod(tgCfg.SessionPath, 0600)
			return nil
		}

		l := login{
			auth:     client.Auth(),
			qr:       client.QR(),
			loggedIn: loggedIn,
			reader:   bufio.NewReader(os.Stdin),
			out:      os.Stdout,
		}
		if err := l.start(ctx); err != nil {
			return err
		}

		_ = os.Chmod(tgCfg.SessionPath, 0600)
		fmt.Println("Authentication successful!")
		fmt.Printf("Session saved to: %s\n", tgCfg.SessionPath)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// Commands typed instead of the login code: resendCommand asks Telegram to
// send the code again via the next delivery method (usually SMS or a call),
// qrCommand switches to QR login.
const (
	resendCommand = "resend"
	qrCommand     = "qr"
)

// qrTimeout bounds how long QR login waits for the code to be scanned.
const qrTimeout = 5 * time.Minute

// login drives interactive sign-in by a phone code or by a QR code.
type login struct {
	auth     *auth.Client
	qr       qrlogin.QR
	loggedIn qrlogin.LoggedIn
	reader   *bufio.Reader
	out      io.Writer
	// readPassword reads a line without echo; nil means the terminal on stdin.
	readPassword func() ([]byte, error)
}

func (l login) readPasswordOrTerm() ([]byte, error) {
	if l.readPassword != nil {
		return l.readPassword()
	}
	return term.ReadPassword(int(syscall.Stdin))
}

// readLine reads one trimmed line. Ctrl-C cancels ctx but cannot interrupt the
// blocking read, so ctx is checked after it.
func (l login) readLine(ctx context.Context) (string, error) {
	line, readErr := l.reader.ReadString('\n')
	if err := ctx.Err(); err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" && readErr != nil {
		return "", fmt.Errorf("read input: %w", readErr)
	}
	return line, nil
}

// start asks how to log in and runs the chosen method. QR comes first and is
// the default: it needs no login code, which Telegram may not deliver to
// third-party apps.
func (l login) start(ctx context.Context) error {
	fmt.Fprintln(l.out, "How do you want to log in?")
	fmt.Fprintln(l.out, "  1) QR code: scan it in a Telegram app where you are already logged in (recommended)")
	fmt.Fprintln(l.out, "  2) Phone number: enter a login code sent by Telegram")
	for {
		fmt.Fprint(l.out, "Choose 1 or 2 [1]: ")
		choice, err := l.readLine(ctx)
		if err != nil {
			return err
		}
		switch choice {
		case "", "1":
			return l.withQR(ctx)
		case "2":
			fmt.Fprint(l.out, "Phone number (e.g. +79001234567): ")
			phone, err := l.readLine(ctx)
			if err != nil {
				return err
			}
			if phone == "" {
				return fmt.Errorf("phone number is required")
			}
			return l.withCode(ctx, phone)
		}
	}
}

// withCode logs in by phone code. It is used instead of auth.Flow so the
// code can be requested again via auth.resendCode: Flow keeps the
// phone_code_hash of the first auth.sendCode and cannot switch to a new one.
func (l login) withCode(ctx context.Context, phone string) error {
	sent, err := l.auth.SendCode(ctx, phone, auth.SendCodeOptions{})
	if err != nil {
		return err
	}
	for {
		var s *tg.AuthSentCode
		switch v := sent.(type) {
		case *tg.AuthSentCode:
			s = v
		case *tg.AuthSentCodeSuccess:
			if _, ok := v.Authorization.(*tg.AuthAuthorization); ok {
				return nil // Telegram authorized the session without a code.
			}
			return fmt.Errorf("sign up not supported")
		default:
			return fmt.Errorf("unexpected sent code type: %T", sent)
		}

		info, err := describeSentCode(s)
		if err != nil {
			return err
		}
		fmt.Fprintln(l.out, info)
		if next := describeNextCodeType(s.NextType); next != "" {
			hint := fmt.Sprintf("If it does not arrive, type %q to get a new code via %s", resendCommand, next)
			if timeout, ok := s.GetTimeout(); ok {
				hint += fmt.Sprintf(" (available in %ds)", timeout)
			}
			fmt.Fprintln(l.out, hint+".")
		}
		fmt.Fprintf(l.out, "Type %q to log in with a QR code instead.\n", qrCommand)

		command, err := l.enterCode(ctx, phone, s.PhoneCodeHash)
		if err != nil {
			return err
		}
		switch command {
		case "":
			return nil
		case qrCommand:
			return l.withQR(ctx)
		}
		next, err := l.auth.ResendCode(ctx, phone, s.PhoneCodeHash)
		if tgerr.Is(err, "SEND_CODE_UNAVAILABLE") {
			fmt.Fprintln(l.out, "Telegram has no other way to send the code; enter the code already sent.")
			continue
		}
		if err != nil {
			return err
		}
		sent = next
	}
}

// enterCode prompts for the login code until sign-in succeeds, asking again
// on a mistyped code. It returns resendCommand or qrCommand when the user
// types one, and "" once signed in.
func (l login) enterCode(ctx context.Context, phone, codeHash string) (string, error) {
	for {
		// Ctrl-C cancels ctx but cannot interrupt the blocking read: stop at
		// the next line instead of prompting again.
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fmt.Fprint(l.out, "Enter code: ")
		line, readErr := l.reader.ReadString('\n')
		if err := ctx.Err(); err != nil {
			return "", err
		}
		code := strings.TrimSpace(line)
		if code == resendCommand || code == qrCommand {
			return code, nil
		}
		if code == "" {
			if readErr != nil {
				return "", fmt.Errorf("read code: %w", readErr)
			}
			continue
		}

		_, err := l.auth.SignIn(ctx, phone, code, codeHash)
		switch {
		case err == nil:
			return "", nil
		case errors.Is(err, auth.ErrPasswordAuthNeeded):
			return "", l.enterPassword(ctx)
		case tgerr.Is(err, "PHONE_CODE_INVALID"):
			fmt.Fprintln(l.out, "Invalid code, try again.")
		case tgerr.Is(err, "PHONE_CODE_EXPIRED"):
			fmt.Fprintf(l.out, "The code has expired; type %q to get a new one.\n", resendCommand)
		default:
			return "", err
		}
	}
}

// withQR logs in by a QR code scanned in a Telegram app where the account is
// already logged in. No login code is involved, which works around Telegram
// not delivering codes to third-party apps. The QR code is a short-lived
// login token, so it is only drawn on the terminal.
func (l login) withQR(ctx context.Context) error {
	qrCtx, cancel := context.WithTimeout(ctx, qrTimeout)
	defer cancel()

	_, err := l.qr.Auth(qrCtx, l.loggedIn, func(_ context.Context, token qrlogin.Token) error {
		code, err := qr.Encode(token.URL(), qr.M)
		if err != nil {
			return err
		}
		fmt.Fprintf(l.out, "\nOn your phone open Telegram → Settings → Devices → Link Desktop Device and scan this code (valid %s):\n\n",
			time.Until(token.Expires()).Round(time.Second))
		fmt.Fprint(l.out, renderQR(code))
		return nil
	})
	if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
		return l.enterPassword(ctx)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("QR code was not scanned within %s", qrTimeout)
	}
	return err
}

// enterPassword prompts for the 2FA password, asking again on a wrong one.
func (l login) enterPassword(ctx context.Context) error {
	for {
		fmt.Fprint(l.out, "Enter 2FA password: ")
		password, err := l.readPasswordOrTerm()
		fmt.Fprintln(l.out) // newline after hidden input
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		_, err = l.auth.Password(ctx, string(password))
		if !errors.Is(err, auth.ErrPasswordInvalid) {
			return err
		}
		fmt.Fprintln(l.out, "Invalid password, try again.")
	}
}

// renderQR draws each module as two spaces with a background color, so the
// cells fill the whole line height (half-block glyphs leave gaps between
// lines). It uses 256-color palette entries 16 (black) and 231 (white):
// terminal themes remap only the 16 basic colors, which washed out the
// contrast. A quiet zone keeps it scannable on dark terminals.
func renderQR(code *qr.Code) string {
	const quiet = 4
	const (
		white = "\x1b[48;5;231m  "
		black = "\x1b[48;5;16m  "
	)
	n := code.Size + 2*quiet

	var b strings.Builder
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if code.Black(x-quiet, y-quiet) {
				b.WriteString(black)
			} else {
				b.WriteString(white)
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String()
}

// describeSentCode tells the user where Telegram delivered the login code.
// By default the code goes to the "Telegram" service chat in an app where the
// account is already logged in, not to SMS, which is easy to miss.
func describeSentCode(s *tg.AuthSentCode) (string, error) {
	var where string
	switch t := s.Type.(type) {
	case *tg.AuthSentCodeTypeApp:
		where = fmt.Sprintf(`the "Telegram" service chat in your logged-in Telegram apps (%d digits)`, t.Length)
	case *tg.AuthSentCodeTypeSMS:
		where = fmt.Sprintf("SMS (%d digits)", t.Length)
	case *tg.AuthSentCodeTypeSMSWord:
		where = "SMS with a code word"
	case *tg.AuthSentCodeTypeSMSPhrase:
		where = "SMS with a code phrase"
	case *tg.AuthSentCodeTypeFirebaseSMS:
		where = fmt.Sprintf("SMS (%d digits)", t.Length)
	case *tg.AuthSentCodeTypeCall:
		where = fmt.Sprintf("a phone call dictating the code (%d digits)", t.Length)
	case *tg.AuthSentCodeTypeFlashCall:
		where = "a flash call: the code is the calling number"
	case *tg.AuthSentCodeTypeMissedCall:
		where = fmt.Sprintf("a missed call: the code is the last %d digits of the number starting with %s", t.Length, t.Prefix)
	case *tg.AuthSentCodeTypeEmailCode:
		where = fmt.Sprintf("email %s (%d digits)", t.EmailPattern, t.Length)
	case *tg.AuthSentCodeTypeFragmentSMS:
		where = fmt.Sprintf("Fragment: %s (%d digits)", t.URL, t.Length)
	case *tg.AuthSentCodeTypeSetUpEmailRequired:
		return "", fmt.Errorf("telegram requires a login email to be set up first; set it in an official Telegram app, then run auth again")
	default:
		where = fmt.Sprintf("an unknown channel (%T)", t)
	}

	return "Code sent to " + where + ".", nil
}

func describeNextCodeType(t tg.AuthCodeTypeClass) string {
	switch t.(type) {
	case *tg.AuthCodeTypeSMS:
		return "SMS"
	case *tg.AuthCodeTypeCall:
		return "a phone call"
	case *tg.AuthCodeTypeFlashCall:
		return "a flash call"
	case *tg.AuthCodeTypeMissedCall:
		return "a missed call"
	case *tg.AuthCodeTypeFragmentSMS:
		return "Fragment"
	default:
		return ""
	}
}

// --- dialogs command ---

// runDialogs prints every dialog with its typed reference, ignoring the ACL,
// so the user can choose which chats to whitelist. It needs only the
// telegram section of the config and an authenticated session.
func runDialogs() {
	configPath := findFlag("--config", os.Args[2:])
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config flag is required")
		os.Exit(1)
	}

	tgCfg, err := config.LoadTelegram(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	limiter := ratelimit.New(config.RateConfig{
		RequestsPerSecond: config.DefaultRequestsPerSecond,
		Burst:             config.DefaultBurst,
	})
	client := tgclient.New(*tgCfg, limiter)
	if err := client.Start(ctx); err != nil {
		if auth.IsUnauthorized(err) {
			fmt.Fprintf(os.Stderr, "error: %v (run `mcp-telegram auth` first)\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
	defer client.Stop()

	self, err := client.Peers().Self(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved Messages: user:%d\n\n", self.ID())

	fmt.Printf("%-11s %-22s %s\n", "TYPE", "REF", "TITLE")
	err = tgclient.ListAllDialogs(ctx, client.API(), func(ref, peerType, name string) {
		fmt.Printf("%-11s %-22s %s\n", peerType, ref, sanitizeTerminal(name))
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: list dialogs: %v\n", err)
		os.Exit(1)
	}
}

// sanitizeTerminal replaces control characters and bidi overrides in s with
// U+FFFD. Chat titles are set by other people, and raw escape sequences in
// them could rewrite or spoof the terminal output.
func sanitizeTerminal(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return unicode.ReplacementChar
		}
		return r
	}, s)
}

// --- serve command ---

func runServe() {
	configPath := findFlag("--config", os.Args[2:])
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config flag is required")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogging(cfg.Logging)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	checker, err := acl.NewChecker(cfg.ACL)
	if err != nil {
		logger.Error("failed to initialize ACL", "error", err)
		os.Exit(1)
	}

	limiter := ratelimit.New(cfg.Limits.Rate)

	client := tgclient.New(cfg.Telegram, limiter)
	logger.Info("connecting to Telegram...")
	if err := client.Start(ctx); err != nil {
		logger.Error("failed to start Telegram client", "error", err)
		os.Exit(1)
	}
	defer client.Stop()
	logger.Info("connected to Telegram")

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-telegram",
		Version: "0.1.0",
	}, nil)

	deps := &tools.Deps{
		Resolver:   &peerResolver{c: client},
		API:        client.API(),
		ACL:        checker,
		Limits:     cfg.Limits,
		Media:      cfg.Media,
		Transcribe: cfg.Transcribe,
	}
	tools.Register(server, deps)

	logger.Info("MCP server starting on stdio")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		logger.Error("MCP server error", "error", err)
		os.Exit(1)
	}
}

// peerResolver adapts *tgclient.Client to the tools.PeerResolver interface.
type peerResolver struct {
	c *tgclient.Client
}

func (r *peerResolver) ResolvePeerForTool(ctx context.Context, ref string) (tools.Peer, acl.PeerIdentity, error) {
	return r.c.ResolvePeerForTool(ctx, ref)
}

// --- helpers ---

func findFlag(name string, args []string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func setupLogging(cfg config.LoggingConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	writer := os.Stderr
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot open log file %s: %v, using stderr\n", cfg.File, err)
		} else {
			writer = f
		}
	}

	logger := slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}
