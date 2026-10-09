"""Offline regression checks for the Kind driver's bounded HTTP assertions."""

import unittest
from unittest.mock import Mock, patch

import app_e2e


class WaitHTTPStatusTests(unittest.TestCase):
    def test_waits_for_both_authoritative_status_and_code(self):
        client = Mock()
        client.exchange.side_effect = [
            (200, b"", {"data": []}),
            (503, b"", {"code": "AUTHORIZATION_UNAVAILABLE"}),
            (403, b"", {"code": "FORBIDDEN"}),
        ]
        with patch.object(app_e2e.time, "monotonic", side_effect=[0, 0, 0.5, 1]), patch.object(
            app_e2e.time, "sleep"
        ):
            app_e2e.wait_http_status(client, "GET", "/api/v1/pods?limit=1", 403, 10, "FORBIDDEN")
        self.assertEqual(client.exchange.call_count, 3)

    def test_mismatch_reports_last_response_without_accepting_it(self):
        for status, code in ((200, "FORBIDDEN"), (403, "AUTHORIZATION_UNAVAILABLE"), (503, "FORBIDDEN")):
            with self.subTest(status=status, code=code):
                client = Mock()
                client.exchange.return_value = (status, b"", {"code": code})
                with patch.object(app_e2e.time, "monotonic", side_effect=[0, 0, 1]), patch.object(
                    app_e2e.time, "sleep"
                ), self.assertRaises(app_e2e.E2EFailure) as failure:
                    app_e2e.wait_http_status(client, "GET", "/api/v1/pods?limit=1", 403, 1, "FORBIDDEN")
                self.assertIn("HTTP 403 with code FORBIDDEN", str(failure.exception))
                self.assertIn(f"last response HTTP {status}/{code}", str(failure.exception))
                self.assertEqual(client.exchange.call_count, 1)

    def test_timeout_diagnostic_omits_untrusted_payload(self):
        private_text = "synthetic-upstream-content-must-stay-private"
        client = Mock()
        client.exchange.return_value = (503, private_text.encode(), {"code": private_text, "message": private_text})
        with patch.object(app_e2e.time, "monotonic", side_effect=[0, 0, 1]), patch.object(
            app_e2e.time, "sleep"
        ), self.assertRaises(app_e2e.E2EFailure) as failure:
            app_e2e.wait_http_status(client, "GET", "/api/v1/pods?limit=1", 403, 1, "FORBIDDEN")
        self.assertIn("last response HTTP 503/UNKNOWN", str(failure.exception))
        self.assertNotIn(private_text, str(failure.exception))


if __name__ == "__main__":
    unittest.main()
