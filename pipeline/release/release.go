package release

import (
	"context"
	"log"
	"os"
	"os/exec"

	"github.com/google/go-github/github"
	"github.com/goreleaser/releaser/config"
	"github.com/goreleaser/releaser/split"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
)

// Pipe for github release
type Pipe struct{}

// Name of the pipe
func (Pipe) Name() string {
	return "GithubRelease"
}

// Run the pipe
func (Pipe) Run(config config.ProjectConfig) error {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.Token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)

	r, err := getOrCreateRelease(client, config)
	if err != nil {
		return err
	}
	var g errgroup.Group
	for _, system := range config.Build.Oses {
		for _, arch := range config.Build.Arches {
			system := system
			arch := arch
			g.Go(func() error {
				return upload(client, *r.ID, system, arch, config)
			})
		}
	}
	return g.Wait()
}

func getOrCreateRelease(client *github.Client, config config.ProjectConfig) (*github.RepositoryRelease, error) {
	owner, repo := split.OnSlash(config.Repo)
	data := &github.RepositoryRelease{
		Name:    github.String(config.Git.CurrentTag),
		TagName: github.String(config.Git.CurrentTag),
		Body:    github.String(description(config.Git.Diff)),
	}
	r, res, err := client.Repositories.GetReleaseByTag(owner, repo, config.Git.CurrentTag)
	if err != nil && res.StatusCode == 404 {
		log.Println("Creating release", config.Git.CurrentTag, "on", config.Repo, "...")
		r, _, err = client.Repositories.CreateRelease(owner, repo, data)
		return r, err
	}
	log.Println("Updating existing release", config.Git.CurrentTag, "on", config.Repo, "...")
	r, _, err = client.Repositories.EditRelease(owner, repo, *r.ID, data)
	return r, err
}

func description(diff string) string {
	result := "## Changelog\n" + diff + "\n\n--\nAutomated with @goreleaser"
	cmd := exec.Command("go", "version")
	bts, err := cmd.CombinedOutput()
	if err != nil {
		return result
	}
	return result + "\nBuilt with " + string(bts)
}

func upload(client *github.Client, releaseID int, system, arch string, config config.ProjectConfig) error {
	owner, repo := split.OnSlash(config.Repo)
	name, err := config.ArchiveName(system, arch)
	if err != nil {
		return err
	}
	name = name + "." + config.Archive.Format
	file, err := os.Open("dist/" + name)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	log.Println("Uploading", file.Name(), "...")
	_, _, err = client.Repositories.UploadReleaseAsset(
		owner,
		repo,
		releaseID,
		&github.UploadOptions{Name: name},
		file,
	)
	return err
}