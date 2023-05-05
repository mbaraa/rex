package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/joho/godotenv"
)

func rexKey() string         { return os.Getenv("REX_KEY") }
func reposDir() string       { return os.Getenv("REPOS_DIR") }
func portNumber() string     { return os.Getenv("PORT") }
func allowedClients() string { return os.Getenv("ALLOWED_CLIENTS") }

func deployRepo(repoName string) error {
	repoDirectory := fmt.Sprintf("%s/%s", reposDir(), repoName)

	pull := exec.Command("git", "pull")
	pull.Dir = repoDirectory
	err := pull.Run()
	if err != nil {
		return err
	}

	composeDown := exec.Command("docker", "compose", "down", "--volumes", "--rmi", "local")
	composeDown.Dir = repoDirectory
	err = composeDown.Run()
	if err != nil {
		return err
	}

	composeUp := exec.Command("docker", "compose", "up", "-d")
	composeUp.Dir = repoDirectory
	err = composeUp.Run()
	if err != nil {
		return err
	}

	return nil
}

func handleDeployRepo(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/json; charset=UTF-8")
	res.Header().Set("Access-Control-Allow-Origin", allowedClients())

	token := req.Header.Get("Authorization")
	if token != rexKey() {
		res.WriteHeader(http.StatusUnauthorized)
		return
	}

	repoName := req.URL.Query().Get("name")
	if len(repoName) == 0 {
		res.WriteHeader(http.StatusBadRequest)
		return
	}

	err := deployRepo(repoName)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		panic(err)
	}
	http.HandleFunc("/deploy/", handleDeployRepo)
	http.ListenAndServe(":"+portNumber(), nil)
}
