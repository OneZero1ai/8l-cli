package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
)

type doctorFlags struct {
	Profile   string
	ConfigDir string
	Verbose   bool
}

func newDoctorCmd() *cobra.Command {
	f := &doctorFlags{}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose binding issues (DNS, TLS, auth, MCP server health)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile to diagnose")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	return cmd
}

// runDoctor never returns a non-zero exit on the first failed check — it
// runs every check and reports them all, so the operator gets the full
// picture. Final exit code is 1 if any check failed, 0 otherwise.
func runDoctor(stdout, stderr io.Writer, f *doctorFlags) error {
	failed := 0
	pass := func(label, detail string) {
		fmt.Fprintf(stdout, "[ ok ] %-22s %s\n", label, detail)
	}
	fail := func(label string, err error) {
		fmt.Fprintf(stdout, "[FAIL] %-22s %s\n", label, err)
		failed++
	}
	skip := func(label, why string) {
		fmt.Fprintf(stdout, "[skip] %-22s %s\n", label, why)
	}

	p, exists, mr, err := profile.Read(f.ConfigDir, f.Profile)
	if mr.Warning != "" {
		fmt.Fprintf(stdout, "[warn] schema version       %s\n", mr.Warning)
	}
	if err != nil && !exists {
		fail("profile read", err)
		fmt.Fprintf(stdout, "\nDoctor finished with %d failure(s)\n", failed)
		return wrapCoded(ExitUnexpected, fmt.Errorf("doctor: profile unreadable"))
	}
	if !exists {
		fail("profile read", fmt.Errorf("profile %q not found", f.Profile))
		fmt.Fprintf(stdout, "\nDoctor finished with %d failure(s)\n", failed)
		return wrapCoded(ExitMissingArg, fmt.Errorf("doctor: no profile"))
	}
	if err != nil {
		fail("profile validate", err)
	} else {
		pass("profile read", fmt.Sprintf("v%d managed_by=%s", p.Version, p.ManagedBy))
	}

	cq := p.MCPServers["cq"]
	endpoint := cq.Env["CQ_ADDR"]
	apiKey := cq.Env["CQ_API_KEY"]

	u, perr := url.Parse(endpoint)
	if perr != nil || u.Host == "" {
		fail("endpoint parse", fmt.Errorf("%q invalid: %v", endpoint, perr))
		// Without a host we can't run network checks.
		fmt.Fprintf(stdout, "\nDoctor finished with %d failure(s)\n", failed)
		return wrapCoded(ExitUnexpected, fmt.Errorf("doctor: endpoint invalid"))
	}
	pass("endpoint parse", endpoint)

	host := u.Hostname()

	// DNS check.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	addrs, derr := net.DefaultResolver.LookupHost(ctx, host)
	if derr != nil {
		fail("DNS", derr)
	} else {
		pass("DNS", fmt.Sprintf("%s → %v", host, addrs))
	}

	// TLS handshake check.
	if u.Scheme == "https" {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		port := u.Port()
		if port == "" {
			port = "443"
		}
		conn, terr := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, port), &tls.Config{
			ServerName: host,
		})
		if terr != nil {
			fail("TLS", terr)
		} else {
			cert := conn.ConnectionState().PeerCertificates
			subj := "<no certs>"
			if len(cert) > 0 {
				subj = cert[0].Subject.CommonName
			}
			pass("TLS", fmt.Sprintf("CN=%s", subj))
			_ = conn.Close()
		}
	} else {
		skip("TLS", "endpoint is not https")
	}

	// /auth/me.
	client := l2client.New(endpoint, apiKey)
	if f.Verbose {
		client.Verbose = newVerboseLogger(stderr, true)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()
	me, aerr := client.AuthMe(ctx2)
	if aerr != nil {
		fail("/auth/me", aerr)
	} else {
		pass("/auth/me", fmt.Sprintf("enterprise=%s l2=%s persona=%s",
			valueOr(me.EnterpriseID, p.Binding.Enterprise),
			valueOr(me.GroupID, p.Binding.L2),
			valueOr(me.Persona, p.Binding.Persona)))

		// Tenant cross-check.
		if me.EnterpriseID != "" && me.EnterpriseID != p.Binding.Enterprise {
			fail("tenant match",
				fmt.Errorf("auth/me enterprise=%q ≠ profile=%q", me.EnterpriseID, p.Binding.Enterprise))
		} else if me.GroupID != "" && me.GroupID != p.Binding.L2 {
			fail("tenant match",
				fmt.Errorf("auth/me l2=%q ≠ profile=%q", me.GroupID, p.Binding.L2))
		} else {
			pass("tenant match", "ok")
		}
	}

	// /openapi.json reachability — sanity for Phase 1 routing.
	openapiURL := fmt.Sprintf("%s://%s/openapi.json", u.Scheme, u.Host)
	req, _ := http.NewRequestWithContext(ctx2, http.MethodGet, openapiURL, nil)
	resp, oerr := http.DefaultClient.Do(req)
	switch {
	case oerr != nil:
		fail("openapi reach", oerr)
	case resp.StatusCode >= 400:
		fail("openapi reach", fmt.Errorf("status %s", resp.Status))
	default:
		pass("openapi reach", fmt.Sprintf("%s", resp.Status))
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	// MCP server reachability — best-effort: just verify the binary exists.
	if _, lerr := exec.LookPath(cq.Command); lerr != nil {
		fail("cq command on PATH", fmt.Errorf("%s: %v", cq.Command, lerr))
	} else {
		pass("cq command on PATH", cq.Command)
	}

	if failed > 0 {
		fmt.Fprintf(stdout, "\nDoctor finished with %d failure(s)\n", failed)
		return wrapCoded(ExitUnexpected, fmt.Errorf("%d check(s) failed", failed))
	}
	fmt.Fprintf(stdout, "\nDoctor: all checks passed\n")
	return nil
}
