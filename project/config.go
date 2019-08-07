package project

import (
	"fmt"
	"strings"

	gerrit "github.com/andygrunwald/go-gerrit"
	"github.com/go-ini/ini"
	llog "github.com/levenlabs/go-llog"
)

var (
	projectConfigPath   = "project.config"
	projectConfigBranch = "refs/meta/config"
	configPluginName    = "slack-integration"
)

// Config represents a slack-integration plugin configuration
type Config struct {
	Enabled                  bool   `ini:"enabled"`
	WebhookURL               string `ini:"webhookurl"`
	Channel                  string `ini:"channel"`
	Username                 string `ini:"username"`
	IgnoreCommitMessage      string `ini:"ignore"`
	IgnoreAuthors            string `ini:"ignore-authors"`
	IgnoreUnchangedPatchSet  bool   `ini:"ignore-unchanged-patch-set"`
	IgnoreWipPatchSet        bool   `ini:"ignore-wip-patch-set"`
	IgnorePrivatePatchSet    bool   `ini:"ignore-private-patch-set"`
	IgnoreOnlyLabels         string `ini:"ignore-only-labels"`
	PublishOnChangeMerged    bool   `ini:"publish-on-change-merged"`
	PublishOnCommentAdded    bool   `ini:"publish-on-comment-added"`
	PublishOnPatchSetCreated bool   `ini:"publish-on-patch-set-created"`
	PublishOnReviewerAdded   bool   `ini:"publish-on-reviewer-added"`
	// PublishPatchSetReviewersAdded controls whether we publish when a reviewer
	// is added as part of uploading a new patch-set. This is only necessary
	// because https://bugs.chromium.org/p/gerrit/issues/detail?id=10042
	PublishPatchSetReviewersAdded bool `ini:"publish-patch-set-reviewers-added"`

	// PublishPatchSetCreatedImmediately changes the patch-set-created event to fire
	// immediately against slack instead of waiting 5 seconds before publishing to
	// collect any automatically added reviewers. This is necessary because of the same
	// bug as above.
	PublishPatchSetCreatedImmediately bool `ini:"publish-patch-set-created-immediately"`

	// PublishOnWipReady and PublishOnPrivateToPublic default to the value of
	// PublishOnPatchSetCreated but since we can't determine if they were false
	// or not set, we use a pointer and then fill in the regular values later
	OrigPublishOnWipReady      *bool `ini:"publish-on-wip-ready"`
	OrigPublishOnPrivatePublic *bool `ini:"publish-on-private-to-public"`
	PublishOnWipReady          bool
	PublishOnPrivateToPublic   bool
}

// DefaultConfig returns a config struct with defaults set
func DefaultConfig() Config {
	return Config{
		Channel:                 "general",
		Username:                "gerrit",
		IgnoreUnchangedPatchSet: true,
		IgnoreWipPatchSet:       true,
		IgnorePrivatePatchSet:   true,
	}
}

func encodeBranch(branch string) string {
	return strings.TrimPrefix(branch, "/refs/heads/")
}

// LoadConfig loads the config for the sent project
func LoadConfig(client *gerrit.Client, project string) (Config, error) {
	cfg := DefaultConfig()
	projects := []string{project}
	// first get a list of all of the parents
	for {
		parent, _, err := client.Projects.GetProjectParent(project)
		if err != nil {
			return cfg, err
		}
		if parent == "" {
			break
		}
		projects = append(projects, parent)
		project = parent
	}
	// now loop through that list backwards and build config
	for i := len(projects) - 1; i >= 0; i-- {
		contents, _, err := client.Projects.GetBranchContent(
			projects[i],
			encodeBranch(projectConfigBranch),
			projectConfigPath,
		)
		if err != nil {
			return cfg, llog.ErrWithKV(err, llog.KV{"project": projects[i]})
		}
		c, err := ini.Load([]byte(contents))
		if err != nil {
			return cfg, llog.ErrWithKV(err, llog.KV{"project": projects[i]})
		}
		if err = c.Section(fmt.Sprintf(`plugin "%s"`, configPluginName)).MapTo(&cfg); err != nil {
			return cfg, err
		}
	}

	// now correct the wip-ready and public-to-private
	if cfg.OrigPublishOnWipReady == nil {
		cfg.PublishOnWipReady = cfg.PublishOnPatchSetCreated
	} else {
		cfg.PublishOnWipReady = *cfg.OrigPublishOnWipReady
	}
	if cfg.OrigPublishOnPrivatePublic == nil {
		cfg.PublishOnPrivateToPublic = cfg.PublishOnPatchSetCreated
	} else {
		cfg.PublishOnPrivateToPublic = *cfg.OrigPublishOnPrivatePublic
	}
	return cfg, nil
}
