# Rex

Redeploy docker compose containers after git push

### Installing the server:

This will install Rex's server which will allow deploying your app

1.  You can use the latest binary release in the [releases](https://github.com/mbaraa/rex/releases)
2.  Or if that doesn't work, compile it yourself using
    *   `go build -ldflags="-w -s"`
3.  Create a `.env` file by copying the example
    *   `cp .env.example .env` if you run windows on your server I have nothing to say to you, except WHY!!
4.  Modify the fields as you like, make sure to not share your token, since you'll use it in the action to deploy the repo, and it being public is not good news :)
5.  Install the systemd service, since I haven't figured out how to make this fully work in docker :(

<!---->

    # /etc/systemd/system/rex.service
    [Unit]
    Description=Rex 

    [Service]
    Type=simple
    User=yourusername # so that git will work, and the other docker stuff
    WorkingDirectory=/path/to/rex/binary/and/env/file # they must be in the same directory, otherwise it won't work :)
    ExecStart=/path/to/rex/binary
    Restart=always

    [Install]
    WantedBy=multi-user.target

6\. Reload systemd's daemons and start the server

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now rex
```

### Adding the GitHub action

This where the fun begins.

1.  Add a [secret](https://docs.github.com/en/actions/security-guides/encrypted-secrets) to your GitHub Repo called `REX_KEY` with the same value that you've set on the server
    *   You can add the server url as a secret as well if you prefer to :)
2.  Create the GitHub action under `.github/workflows/rex-build.yml`

```yaml
jobs:
  rex-deploy:
    runs-on: ubuntu-latest
    steps:
      - name: rex-7567-e27
        uses: mbaraa/rex-action@v1.0
        with:
          server-url: example.com
          token: ${{ secrets.REX_KEY }}
          repo-name: repoName
```

3.  Do a push and see the magic happen
