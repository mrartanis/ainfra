#!/usr/bin/env python3
"""One-time operator provisioning; this helper is not part of the probe runtime."""
import argparse
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import pwd
import re
import secrets
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def rci_error(value):
    if isinstance(value, dict):
        return (value.get("status") == "error" and "code" in value) or any(map(rci_error, value.values()))
    return isinstance(value, list) and any(map(rci_error, value))


class Client:
    def __init__(self, url, ca_file):
        endpoint = urllib.parse.urlsplit(url)
        if endpoint.scheme != "https" or not endpoint.hostname or endpoint.username or endpoint.password or endpoint.query or endpoint.fragment or endpoint.path not in ("", "/"):
            raise RuntimeError("invalid_https_endpoint")
        self.url = url.rstrip("/")
        self.opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}), NoRedirect(),
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
            urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=ca_file)))

    def request(self, path, data=None):
        req = urllib.request.Request(self.url + path,
            data=None if data is None else json.dumps(data).encode(),
            headers={"Content-Type": "application/json"})
        try:
            response = self.opener.open(req, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read((2 << 20) + 1)
            if len(raw) > 2 << 20:
                raise RuntimeError("response_limit")
            try:
                body = json.loads(raw)
            except (ValueError, UnicodeError):
                body = None
            return response.status, response.headers, body

    def call(self, path, data=None):
        status, _, body = self.request(path, data)
        if status != 200 or body is None or rci_error(body):
            raise RuntimeError("rci_request_failed")
        return body

    def login(self, username, password):
        status, headers, _ = self.request("/auth")
        self.realm = headers.get("X-NDM-Realm")
        challenge = headers.get("X-NDM-Challenge")
        if status != 401 or not self.realm or not challenge:
            raise RuntimeError("unsupported_auth_challenge")
        first = hashlib.md5((username + ":" + self.realm + ":" + password).encode()).hexdigest()
        proof = hashlib.sha256((challenge + first).encode()).hexdigest()
        status, _, _ = self.request("/auth", {"login": username, "password": proof})
        if status != 200:
            raise RuntimeError("auth_rejected_stop_retries")


def verify(url, ca_file, credentials):
    reader = Client(url, ca_file)
    reader.login(credentials["username"], credentials["password"])
    for path in ("version", "system", "interface", "ip/route", "ipv6/route"):
        reader.call("/rci/show/" + path)


def provision(client, url, ca_file, path, owner):
    version = client.call("/rci/show/version")
    match = re.fullmatch(r"(\d+)\.(\d+)\.(\d+)", version.get("title", ""))
    if not match or tuple(map(int, match.groups())) < (5, 0, 4):
        raise RuntimeError("readonly_firmware_not_verified_safe")
    if client.call("/rci/show/last-change").get("fail-safe", {}).get("unsaved") is not False:
        raise RuntimeError("preexisting_unsaved_configuration")
    if not path.is_absolute() or not path.parent.is_dir() or path.is_symlink():
        raise RuntimeError("absolute_private_output_required")
    if path.exists():
        if not path.is_file() or path.stat().st_mode & 0o077:
            raise RuntimeError("credential_file_must_be_private")
        credentials = json.loads(path.read_text())
    else:
        credentials = {"username": "infra_pi_" + secrets.token_hex(4), "password": secrets.token_urlsafe(32)}
        with path.open("x") as out:
            os.fchmod(out.fileno(), 0o600)
            if owner:
                identity = pwd.getpwnam(owner)
                os.fchown(out.fileno(), identity.pw_uid, identity.pw_gid)
            json.dump(credentials, out)
            out.flush()
            os.fsync(out.fileno())
    username = credentials["username"]
    if not re.fullmatch(r"infra_pi_[A-Za-z0-9_]+", username):
        raise RuntimeError("not_a_managed_probe_username")
    current = client.request("/rci/user/tag?name=" + username)
    if current[0] == 200 and isinstance(current[2], list) and set(current[2]) - {"http", "readonly"}:
        raise RuntimeError("existing_user_has_unexpected_privileges")
    digest = hashlib.md5((username + ":" + client.realm + ":" + credentials["password"]).encode()).hexdigest()
    # Set the password before granting access; scalar password fields are ineffective.
    client.call("/rci/", {"user": {"name": username, "password": {"md5": {"hash": digest}}}})
    client.call("/rci/", {"user": {"name": username, "tag": [{"tag": "readonly"}, {"tag": "http"}]}})
    if set(client.call("/rci/user/tag?name=" + username)) != {"readonly", "http"}:
        raise RuntimeError("readonly_tags_unconfirmed")
    verify(url, ca_file, credentials)
    client.call("/rci/system/configuration/save", {})
    if client.call("/rci/show/last-change").get("fail-safe", {}).get("unsaved") is not False:
        raise RuntimeError("configuration_save_unconfirmed")
    return credentials


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", required=True)
    parser.add_argument("--ca-file")
    parser.add_argument("--credential-output", type=Path, required=True)
    parser.add_argument("--owner", help="Owner for a new credential file, e.g. infra-probe")
    parser.add_argument("--admin-login", default="admin")
    parser.add_argument("--verify-only", action="store_true", help="Read using an existing credential file; no admin password or mutations")
    args = parser.parse_args()
    try:
        if args.verify_only:
            if not args.credential_output.is_file() or args.credential_output.stat().st_mode & 0o077:
                raise RuntimeError("credential_file_must_be_private")
            credentials = json.loads(args.credential_output.read_text())
            verify(args.url, args.ca_file, credentials)
        else:
            # One-time admin secret arrives through stdin, never argv or environment.
            password = sys.stdin.buffer.read(4097)
            if not password or len(password) > 4096:
                raise RuntimeError("invalid_admin_password_input")
            client = Client(args.url, args.ca_file)
            client.login(args.admin_login, password.decode().strip())
            credentials = provision(client, args.url, args.ca_file, args.credential_output, args.owner)
        print(json.dumps({"ok": True, "username": credentials["username"], "credential_file": str(args.credential_output)}))
        return 0
    except Exception as error:
        # Provider errors can echo password/hash; never print raw bodies or exceptions.
        code = str(error) if isinstance(error, RuntimeError) else type(error).__name__
        print(json.dumps({"ok": False, "error": code}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
