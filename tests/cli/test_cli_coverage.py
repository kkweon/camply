"""
Test CLI Coverage
"""

from tests.conftest import CamplyRunner, vcr_cassette


def test_test_notifications(cli_runner: CamplyRunner) -> None:
    """
    Test test-notifications command
    """
    test_command = "camply test-notifications --notifications silent"
    result = cli_runner.run_camply_command(command=test_command)
    assert result.exit_code == 0


def test_equipment_types_goingtocamp_missing_recarea(cli_runner: CamplyRunner) -> None:
    test_command = "camply equipment-types --provider GoingToCamp"
    result = cli_runner.run_camply_command(command=test_command)
    assert result.exit_code == 1


def test_campgrounds_missing_args(cli_runner: CamplyRunner) -> None:
    test_command = "camply campgrounds"
    result = cli_runner.run_camply_command(command=test_command)
    assert result.exit_code == 1


def test_equipment_types_recdotgov(cli_runner: CamplyRunner) -> None:
    test_command = "camply equipment-types --provider RecreationDotGov"
    result = cli_runner.run_camply_command(command=test_command)
    assert result.exit_code == 0


def test_campsites_missing_args(cli_runner: CamplyRunner) -> None:
    test_command = "camply campsites"
    result = cli_runner.run_camply_command(command=test_command)
    assert result.exit_code == 1


def test_recreation_areas_missing_args(cli_runner: CamplyRunner) -> None:
    test_command = "camply recreation-areas"
    result = cli_runner.run_camply_command(command=test_command)
    assert result.exit_code == 1


def test_providers_command(cli_runner: CamplyRunner) -> None:
    test_command = "camply providers"
    result = cli_runner.run_camply_command(command=test_command)
    assert result.exit_code == 0


@vcr_cassette
def test_test_notifications_silent(cli_runner: CamplyRunner) -> None:
    test_command = "camply test-notifications --notifications silent"
    result = cli_runner.run_camply_command(command=test_command)
    assert result.exit_code == 0
