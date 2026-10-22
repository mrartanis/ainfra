import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("provision_user", Path(__file__).with_name("provision-user.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class Client:
    realm = "Test Router"

    def __init__(self, version="5.1.6", unsaved=False, fail_password=False):
        self.version, self.unsaved, self.fail_password = version, unsaved, fail_password
        self.writes = []

    def request(self, path):
        return 200, {}, {"status": [{"status": "error", "code": "missing"}]}

    def call(self, path, data=None):
        if data is not None:
            self.writes.append((path, data))
            if self.fail_password:
                raise RuntimeError("rci_request_failed")
            return {}
        if path == "/rci/show/version":
            return {"title": self.version}
        if path == "/rci/show/last-change":
            return {"fail-safe": {"unsaved": self.unsaved}}
        return ["http", "readonly"]


class ProvisioningBoundaries(unittest.TestCase):
    def test_unsafe_firmware_or_unsaved_changes_do_not_mutate(self):
        for client in [Client(version="4.3.6"), Client(version="unknown"), Client(unsaved=True)]:
            with self.subTest(version=client.version, unsaved=client.unsaved), tempfile.TemporaryDirectory() as directory:
                output = Path(directory) / "credentials.json"
                with self.assertRaises(RuntimeError):
                    module.provision(client, "https://router.example", None, output, None)
                self.assertEqual(client.writes, [])
                self.assertFalse(output.exists())

    def test_failed_password_setup_never_grants_access_or_saves(self):
        client = Client(fail_password=True)
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "credentials.json"
            with self.assertRaises(RuntimeError):
                module.provision(client, "https://router.example", None, output, None)
            self.assertEqual(len(client.writes), 1)
            self.assertNotIn("tag", client.writes[0][1]["user"])
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)

    def test_failed_reader_verification_never_saves(self):
        client = Client()
        with tempfile.TemporaryDirectory() as directory, patch.object(module, "verify", side_effect=RuntimeError("auth_rejected")):
            with self.assertRaises(RuntimeError):
                module.provision(client, "https://router.example", None, Path(directory)/"credentials.json", None)
        self.assertFalse(any(path.endswith("/save") for path, _ in client.writes))

    def test_rci_error_in_http_success_is_a_failure(self):
        client = module.Client("https://router.example", None)
        with patch.object(client, "request", return_value=(200, {}, {"status": [{"status": "error", "code": "1", "message": "password echo"}]})):
            with self.assertRaisesRegex(RuntimeError, "^rci_request_failed$"):
                client.call("/rci/", {})


if __name__ == "__main__":
    unittest.main()
