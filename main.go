package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "embed"
)

var (
	rexKey            string
	reposDir          string
	portNumber        string
	allowedOrigins    string
	allowedOriginsMap = map[string]bool{}
	//go:embed repo-configs.json
	repoConfigsFile []byte
	repoConfigs     = sync.OnceValue(func() RepoConfigs {
		configs := map[string]RepoConfig{}
		err := json.Unmarshal(repoConfigsFile, &configs)
		if err != nil {
			log.Panicln(err)
		}

		return RepoConfigs{
			configs: configs,
		}
	})
)

type RepoConfig struct {
	LocalName       string `json:"local_name"`
	ComposeFileName string `json:"compose_file_name"`
}

type RepoConfigs struct {
	configs map[string]RepoConfig
}

func (r RepoConfigs) Get(repoOwner, repoName string) (RepoConfig, error) {
	if conf, ok := r.configs[repoOwner+"/"+repoName]; !ok {
		return RepoConfig{}, fmt.Errorf("repo `%s/%s` not configured", repoOwner, repoName)
	} else {
		return conf, nil
	}
}

func init() {
	flag.StringVar(&rexKey, "rex-key", os.Getenv("REX_AUTH_KEY"), "give me a secure key to use the GitHub action with")
	flag.StringVar(&reposDir, "repos-dir", os.Getenv("REX_REPOS_DIR"), "give me a proper directory path where your GitHub repos are stored in")
	flag.StringVar(&portNumber, "port", getEnv("REX_PORT_NUMBER", "7567"), "give me a port number (default is 7567)")
	flag.StringVar(&allowedOrigins, "allowed-origins", os.Getenv("REX_ALLOWED_ORIGINS"), "give me a list of allowed origins")
	parseAllowedOringins()
}

func main() {
	http.HandleFunc("GET /deploy/github", handleDeployRepoGitHub)
	http.HandleFunc("POST /deploy/codeberg", handleDeployRepoCodeberg)
	log.Printf("Starting http server on port %s\n", portNumber)
	log.Fatalln(http.ListenAndServe(":"+portNumber, nil))
}

func parseAllowedOringins() {
	_allowedOrigins := strings.Split(
		regexp.MustCompile(`\s*,\s*`).ReplaceAllString(allowedOrigins, ","),
		",",
	)
	for _, allowedOrigin := range _allowedOrigins {
		allowedOriginsMap[allowedOrigin] = true
	}
}

////////////////////////////////
// GitHub rex-deploy action
////////////////////////////////

func handleDeployRepoGitHub(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if origin := req.Header.Get("Origin"); allowedOriginsMap[origin] || allowedOriginsMap["*"] {
		res.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
	}

	token := req.Header.Get("Authorization")
	if token != "Bearer "+rexKey {
		res.WriteHeader(http.StatusUnauthorized)
		return
	}

	repoName := req.URL.Query().Get("name")
	if len(repoName) == 0 {
		res.WriteHeader(http.StatusBadRequest)
		return
	}

	commitSha := req.URL.Query().Get("commit_sha")
	latestTag := req.URL.Query().Get("latest_tag")
	composeFileName := req.URL.Query().Get("compose_file_name")

	logsText, err := deployRepo(repoName, commitSha, latestTag, composeFileName)
	if err != nil {
		log.Println(err)
		res.WriteHeader(http.StatusInternalServerError)
		res.Write(logsText)
		return
	}

	res.Write(logsText)
}

////////////////////////////////
// Codeberg forgejo webhook
////////////////////////////////

// this code is a 1:1 translation from the JSON response example by forgejo
// https://forgejo.org/docs/latest/user/webhooks/
//

type CodebergWebhookPayload struct {
	Ref        string             `json:"ref"`
	Before     string             `json:"before"`
	After      string             `json:"after"`
	CompareURL string             `json:"compare_url"`
	Commits    []CodebergCommit   `json:"commits"`
	Repository CodebergRepository `json:"repository"`
	Pusher     CodebergUser       `json:"pusher"`
	Sender     CodebergUser       `json:"sender"`
}

type CodebergCommit struct {
	ID        string         `json:"id"`
	Message   string         `json:"message"`
	URL       string         `json:"url"`
	Author    CodebergAuthor `json:"author"`
	Committer CodebergAuthor `json:"committer"`
	Timestamp time.Time      `json:"timestamp"`
}

type CodebergAuthor struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type CodebergRepository struct {
	ID              int          `json:"id"`
	Owner           CodebergUser `json:"owner"`
	Name            string       `json:"name"`
	FullName        string       `json:"full_name"`
	Description     string       `json:"description"`
	Private         bool         `json:"private"`
	Fork            bool         `json:"fork"`
	HTMLURL         string       `json:"html_url"`
	SSHURL          string       `json:"ssh_url"`
	CloneURL        string       `json:"clone_url"`
	Website         string       `json:"website"`
	StarsCount      int          `json:"stars_count"`
	ForksCount      int          `json:"forks_count"`
	WatchersCount   int          `json:"watchers_count"`
	OpenIssuesCount int          `json:"open_issues_count"`
	DefaultBranch   string       `json:"default_branch"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

type CodebergUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

//
// end of forgejo code

func handleDeployRepoCodeberg(res http.ResponseWriter, req *http.Request) {
	// this code is a 1:1 translation from the PHP example by forgejo
	// https://forgejo.org/docs/latest/user/webhooks/
	//

	if req.Header.Get("Content-Type") != "application/json" {
		res.WriteHeader(http.StatusBadRequest)
		log.Printf("FAILED - not application/json - '. %s", req.Header.Get("Content-Type"))
		return
	}

	headerSignature := req.Header.Get("X-Forgejo-Signature")
	if headerSignature == "" {
		log.Println("FAILED - header signature missing")
		http.Error(res, "Signature missing", http.StatusUnauthorized)
		return
	}

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("FAILED - unable to read body: %v", err)
		http.Error(res, "Bad request", http.StatusBadRequest)
		return
	}

	mac := hmac.New(sha256.New, []byte(rexKey))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(headerSignature), []byte(expectedSignature)) {
		log.Println("FAILED - payload signature mismatch")
		http.Error(res, "Invalid signature", http.StatusForbidden)
		return
	}

	var reqBody CodebergWebhookPayload
	err = json.Unmarshal(payload, &reqBody)
	if err != nil {
		log.Printf("FAILED - json decode - %v\n", err)
		return
	}
	//
	// end of forgejo code

	res.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if origin := req.Header.Get("Origin"); allowedOriginsMap[origin] || allowedOriginsMap["*"] {
		res.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
	}

	token := req.Header.Get("Authorization")
	if token != "Bearer "+rexKey {
		res.WriteHeader(http.StatusUnauthorized)
		return
	}

	repoConfig, err := repoConfigs().Get(reqBody.Repository.Owner.Username, reqBody.Repository.Name)
	if err != nil {
		log.Println(err)
		http.Error(res, "Repo not found!", http.StatusNotFound)
		return
	}

	logsText, err := deployRepo(repoConfig.LocalName, reqBody.Commits[0].ID, "", repoConfig.ComposeFileName)
	if err != nil {
		log.Println(err)
		res.WriteHeader(http.StatusInternalServerError)
		res.Write(logsText)
		return
	}

	res.Write(logsText)
}

////////////////////////////////
// Actual docker deployer mf
////////////////////////////////

func deployRepo(repoName, commitSha, latestTag, composeFileName string) ([]byte, error) {
	repoDirectory := fmt.Sprintf("%s/%s", reposDir, repoName)

	log.Printf("Deploying %s...\n", repoDirectory)

	outBuff := bytes.NewBuffer([]byte{})

	pull := exec.Command("git", "pull")
	pull.Stdout = outBuff
	pull.Dir = repoDirectory
	err := pull.Run()
	if err != nil {
		return outBuff.Bytes(), err
	}

	var build *exec.Cmd
	if composeFileName != "" {
		build = exec.Command("docker", "compose", "-f", composeFileName, "build", "--no-cache")
	} else {
		build = exec.Command("docker", "compose", "build", "--no-cache")
	}

	build.Stdout = outBuff
	build.Dir = repoDirectory
	err = build.Run()
	if err != nil {
		return outBuff.Bytes(), err
	}

	var composeDown *exec.Cmd
	if composeFileName != "" {
		composeDown = exec.Command("docker", "compose", "-f", composeFileName, "down", "--volumes", "--rmi", "local")
	} else {
		composeDown = exec.Command("docker", "compose", "down", "--volumes", "--rmi", "local")
	}

	composeDown.Stdout = outBuff
	composeDown.Dir = repoDirectory
	err = composeDown.Run()
	if err != nil {
		return outBuff.Bytes(), err
	}

	var composeUp *exec.Cmd
	if composeFileName != "" {
		composeUp = exec.Command("docker", "compose", "-f", composeFileName, "up", "-d")
	} else {
		composeUp = exec.Command("docker", "compose", "up", "-d")
	}

	composeUp.Env = append(composeUp.Env, fmt.Sprintf("COMMIT_SHA=%s", commitSha), fmt.Sprintf("LATEST_TAG=%s", latestTag))
	composeUp.Stdout = outBuff
	composeUp.Dir = repoDirectory
	err = composeUp.Run()
	if err != nil {
		return outBuff.Bytes(), err
	}

	log.Printf("Done deploying %s...\n", repoDirectory)

	return outBuff.Bytes(), nil
}

func getEnv(envName, fallbackValue string) string {
	if value := os.Getenv(envName); value != "" {
		return value
	}
	return fallbackValue
}
