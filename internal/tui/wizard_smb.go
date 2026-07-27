package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/thedataflows/dotdrift/internal/generate"
)

// RunSmbWizard runs the interactive smb wizard: tab chrome, the server
// form, the shares loop, and a single WriteModule at the end. Aborting
// (ctrl+c/esc) exits cleanly with nothing written.
func RunSmbWizard(ctx context.Context, p SmbParams) error {
	tab, err := switchTab("smb")
	if err != nil || tab == "quit" {
		return err
	}
	if tab == "mounts" {
		return RunMountsWizard(ctx, MountsParams{
			Profile: p.Profile, Layer: p.Layer, Hostname: p.Hostname, Username: p.Username,
		})
	}
	return runSmbFlow(ctx, p)
}

// runSmbFlow is the smb wizard body once the smb tab is chosen.
func runSmbFlow(_ context.Context, p SmbParams) error {
	sel, err := promptTarget(p.Layer, p.ModuleID, "smb", p.Hostname, p.Username)
	if err != nil {
		return friendlyAbort(err)
	}

	_, _, username, err := invokingUser()
	if err != nil {
		return err
	}

	server, err := promptServer(p, username)
	if err != nil {
		return friendlyAbort(err)
	}

	wizard := NewSmbWizard(sel)
	wizard.SetServer(server)

	prefilled, err := p.PrefillShares()
	if err != nil {
		return err
	}
	for _, c := range prefilled {
		if err := wizard.AddShare(c); err != nil {
			return err
		}
	}

	more := len(prefilled) == 0
	for {
		if !more {
			more, err = confirm("Add a share?", len(prefilled) == 0)
			if err != nil {
				return friendlyAbort(err)
			}
			if !more {
				break
			}
		}
		share, err := promptShare()
		if err != nil {
			return friendlyAbort(err)
		}
		keep, err := confirmShare(share)
		if err != nil {
			return friendlyAbort(err)
		}
		if keep {
			if err := wizard.AddShare(share); err != nil {
				return err
			}
		}
		more = false
	}

	write, err := confirmSmbSpec(sel, server, wizard)
	if err != nil {
		return friendlyAbort(err)
	}
	if !write {
		fmt.Fprintln(os.Stderr, "discarded; nothing written")
		return nil
	}

	uid, gid, _, err := invokingUser()
	if err != nil {
		return err
	}
	if err := wizard.Write(p.Profile, username, uid, gid); err != nil {
		return err
	}
	return printSummary(p.Profile, sel)
}

// promptServer runs the server-level form: group, users
// (comma-separated), and the avahi confirm.
func promptServer(p SmbParams, username string) (SmbServerChoice, error) {
	group := p.Group
	if group == "" {
		group = "smb"
	}
	users := strings.Join(p.Users, ", ")
	if users == "" {
		users = username
	}
	avahi := true
	if p.Avahi != nil {
		avahi = *p.Avahi
	}

	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Samba group").
			Description("valid users = @<group>").
			Value(&group).
			Validate(nonEmpty("group")),
		huh.NewInput().
			Title("Samba users (comma-separated)").
			Value(&users).
			Validate(nonEmpty("users")),
		huh.NewConfirm().
			Title("Avahi service discovery?").
			Value(&avahi),
	)).Run(); err != nil {
		return SmbServerChoice{}, err
	}

	choice := SmbServerChoice{Group: group}
	for _, u := range strings.Split(users, ",") {
		if u = strings.TrimSpace(u); u != "" {
			choice.Users = append(choice.Users, u)
		}
	}
	// A "yes" keeps the key unset (default-on semantics, like the CLI
	// with no flag); only a "no" records an explicit avahi = false.
	if !avahi {
		off := false
		choice.Avahi = &off
	}
	return choice, nil
}

// promptShare runs one share form: name, path, comment, writable, public.
func promptShare() (ShareChoice, error) {
	var c ShareChoice
	c.Writable = true
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Share name").Value(&c.Name).Validate(nonEmpty("name")),
		huh.NewInput().Title("Share path").Placeholder("/srv/media").Value(&c.Path).Validate(nonEmpty("path")),
		huh.NewInput().Title("Comment (optional)").Value(&c.Comment),
		huh.NewConfirm().Title("Writable?").Value(&c.Writable),
		huh.NewConfirm().Title("Public (guest access)?").Value(&c.Public),
	)).Run(); err != nil {
		return ShareChoice{}, err
	}
	return c, nil
}

// confirmShare renders a share review and asks whether to keep it.
func confirmShare(c ShareChoice) (bool, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "name:     %s\n", c.Name)
	fmt.Fprintf(&b, "path:     %s\n", c.Path)
	if c.Comment != "" {
		fmt.Fprintf(&b, "comment:  %s\n", c.Comment)
	}
	fmt.Fprintf(&b, "writable: %t\n", c.Writable)
	fmt.Fprintf(&b, "public:   %t\n", c.Public)

	keep := true
	if err := huh.NewForm(huh.NewGroup(
		huh.NewNote().Title("Review share").Description(b.String()),
		huh.NewConfirm().Title("Keep this share?").Value(&keep),
	)).Run(); err != nil {
		return false, err
	}
	return keep, nil
}

// confirmSmbSpec renders the final review and the write confirmation.
func confirmSmbSpec(sel generate.Selection, server SmbServerChoice, wizard *SmbWizard) (bool, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "module:  %s (layer %s)\n", sel.ModuleID, sel.Layer)
	fmt.Fprintf(&b, "group:   %s\n", server.Group)
	fmt.Fprintf(&b, "users:   %s\n", strings.Join(server.Users, ", "))
	avahi := "yes"
	if server.Avahi != nil && !*server.Avahi {
		avahi = "no"
	}
	fmt.Fprintf(&b, "avahi:   %s\n", avahi)
	fmt.Fprintf(&b, "shares:  %d configured\n", len(wizard.shares))

	write := true
	if err := huh.NewForm(huh.NewGroup(
		huh.NewNote().Title("Review smb module").Description(b.String()),
		huh.NewConfirm().
			Title("Write the module?").
			Description("All shares are written in one pass; declining discards every choice.").
			Value(&write),
	)).Run(); err != nil {
		return false, err
	}
	return write, nil
}
