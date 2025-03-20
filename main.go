package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var (
	rexKey            string
	reposDir          string
	portNumber        string
	allowedOrigins    string
	allowedOriginsMap = map[string]bool{}
)

func init() {
	flag.StringVar(&rexKey, "rex-key", os.Getenv("REX_AUTH_KEY"), "give me a secure key to use the GitHub action with")
	flag.StringVar(&reposDir, "repos-dir", os.Getenv("REX_REPOS_DIR"), "give me a proper directory path where your GitHub repos are stored in")
	flag.StringVar(&portNumber, "port", getEnv("REX_PORT_NUMBER", "7567"), "give me a port number (default is 7567)")
	flag.StringVar(&allowedOrigins, "allowed-origins", os.Getenv("REX_ALLOWED_ORIGINS"), "give me a list of allowed origins")
	parseAllowedOringins()
}

func main() {
	http.HandleFunc("/deploy/", handleDeployRepo)
	http.ListenAndServe(":"+portNumber, nil)
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

func handleDeployRepo(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if origin := req.Header.Get("Origin"); allowedOriginsMap[origin] || allowedOriginsMap["*"] {
		res.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
	}

	token := req.Header.Get("Authorization")
	if token != rexKey {
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

func deployRepo(repoName, commitSha, latestTag, composeFileName string) ([]byte, error) {
	repoDirectory := fmt.Sprintf("%s/%s", reposDir, repoName)

	outBuff := bytes.NewBuffer([]byte{})

	pull := exec.Command("git", "pull")
	pull.Stdout = outBuff
	pull.Dir = repoDirectory
	err := pull.Run()
	if err != nil {
		return outBuff.Bytes(), err
	}

	build := exec.Command("docker", "compose", "build", "--no-cache")
	build.Stdout = outBuff
	build.Dir = repoDirectory
	err = build.Run()
	if err != nil {
		return outBuff.Bytes(), err
	}

	composeDown := exec.Command("docker", "compose", "down", "--volumes", "--rmi", "local")
	composeDown.Stdout = outBuff
	composeDown.Dir = repoDirectory
	err = composeDown.Run()
	if err != nil {
		return outBuff.Bytes(), err
	}

	var composeUp *exec.Cmd
	if composeFileName != "" {
		composeUp = exec.Command("docker", "compose", "up", "-f", composeFileName, "-d")
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

	return outBuff.Bytes(), nil
}

func getEnv(envName, fallbackValue string) string {
	if value := os.Getenv(envName); value != "" {
		return value
	}
	return fallbackValue
}
