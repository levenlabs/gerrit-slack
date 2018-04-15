# gerrit-slack

A tiny service that listens for gerrit events and publishes them to a Slack
webhook.

It acts much like the official [slack-integration](https://gerrit.googlesource.com/plugins/slack-integration/)
plugin.

## Config

See the [slack-integration](https://gerrit.googlesource.com/plugins/slack-integration/)
plugin for docs on how to configure your `project.config`.

In addition, you need to make an ini-formatted config file for `gerrit-slack`
so it knows how to reach your Gerrit instance. The config file looks like:

```
[gerrit]
  http-address = https://mygerrit.com/
  ssh-address = mygerrit.com:2222
  username = slack
  password = my-super-secure-password
  private-key-path = /path/to/a/private/key
  host-key = ecdsa-sha2-nistp521 ...
```

The host-key can be copied from https://gerrit/#/settings/ssh-keys and should
be the string that users typically add to their `known_hosts` file.

## Running

```
gerrit-slack [--config=./slack.config] [--log-level=info]
```
