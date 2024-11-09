# Rex

Redeploy docker compose containers after git push

### Installing the server:

This will install Rex's server which will allow deploying your app

1.  You can use the latest binary release in the [releases](https://github.com/mbaraa/rex/releases)
2.  Or if that doesn't work, compile it yourself using
    - `go build -ldflags="-w -s"`
3.  Set the environmental variables as showm in the `.env.example`
    - Either use flags when running the server, or set the environmental variable manually.
    - Values' are taken from flags, if it doesn't exist it uses the corresponding environmental variable.

| Flag              | Env var               | Description                                                           |
| ----------------- | --------------------- | --------------------------------------------------------------------- |
| `port`            | `REX_PORT_NUMBER`     | Give me a port number. (default: `7567`)                              |
| `rex-key`         | `REX_AUTH_KEY`        | Give me a secure key to use the GitHub action with                    |
| `repos-dir`       | `REX_REPOS_DIR`       | Give me a proper directory path where your GitHub repos are stored in |
| `allowed-origins` | `REX_ALLOWED_ORIGINS` | give me a list of allowed origins                                     |
| `github-username` | `REX_GITHUB_USERNAME` | Give me your GitHub username so I can pull repos' changes             |
| `github-token`    | `REX_GITHUB_TOKEN`    | Give me your GitHub token so I can pull repos' changes                |

4.  Install the systemd service, since I haven't figured out how to make this fully work in docker :(

<!---->

    # /etc/systemd/system/rex.service
    [Unit]
    Description=Rex

    [Service]
    Type=simple
    User=yourusername # REMOVE THIS COMMENT; so that git will work, and the other docker stuff
    EnvironmentFile=/path/to/rex/.env_file # REMOVE THIS COMMENT; let systemd handle parsing the environmental variables
    WorkingDirectory=/path/to/rex/ # REMOVE THIS COMMENT; they must be in the same directory, otherwise it won't work :)
    ExecStart=/path/to/rex/binary
    Restart=always

    [Install]
    WantedBy=multi-user.target

5. Reload systemd's daemons and start the server

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now rex
```

### Adding the GitHub action

This where the fun begins.

1.  Add a [secret](https://docs.github.com/en/actions/security-guides/encrypted-secrets) to your GitHub Repo called `REX_KEY` with the same value that you've set on the server
    - You can add the server url as a secret as well if you prefer to :)
2.  Create the GitHub action under `.github/workflows/rex-build.yml`

```yaml
jobs:
  rex-deploy:
    runs-on: ubuntu-latest
    steps:
      - name: rex-7567-e27
        uses: mbaraa/rex-action@v1.2
        with:
          server-url: example.com
          token: ${{ secrets.REX_KEY }}
          repo-name: repoName
          # commit-sha is optional :)
          commit-sha: ${{ github.sha }}
```

3.  Do a push and see the magic happen
