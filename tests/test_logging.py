import logging
from unittest.mock import patch

from rich.logging import RichHandler

from camply.config.logging_config import set_up_logging


def test_rich_logging_format():
    # Remove any existing handlers
    logging.root.handlers = []

    with patch("camply.config.logging_config.getenv") as mock_getenv:
        # Mock getenv to ensure we use 'rich' handler and not 'python'
        # (PYTEST_CURRENT_TEST can sometimes force python handler)
        def mock_env(key, default=None):
            if key == "PYTEST_CURRENT_TEST":
                return None
            if key == "CAMPLY_LOG_HANDLER":
                return "rich"
            if key == "LOG_LEVEL":
                return "INFO"
            return default

        mock_getenv.side_effect = mock_env

        set_up_logging()

        # Check if the handler is RichHandler
        assert len(logging.root.handlers) == 1
        handler = logging.root.handlers[0]
        assert isinstance(handler, RichHandler)

        # Check if the formatter format matches
        assert handler.formatter is not None
        assert handler.formatter.datefmt == "[%Y-%m-%d %H:%M:%S]"
        assert handler.formatter._fmt == "%(message)s"
