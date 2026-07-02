# Secrets

`~/.pm.yaml` can contain literal API keys (`api_key:`). When displaying it
in a shell command, always redact:

```sh
sed -E 's/(api_key:).*/\1 <REDACTED>/' ~/.pm.yaml
```

`pm _resolve-key <profile>` prints the resolved key to stdout — never run
it bare in a logged session; capture into a shell variable if a request needs
it (`KEY=$(./pm _resolve-key <profile>)`) and pass via headers, never echo.

The config file must keep `0600` permissions (`CheckConfigPermissions` warns
otherwise); preserve that in any code path that writes it.
