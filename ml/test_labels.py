from train import derive_labels


def row(*targets: dict[str, str], **extra: object) -> dict[str, object]:
    return {"status": "inspected", "targets": list(targets), **extra}


def test_universal_mac() -> None:
    r = row(
        {"platform": "macos", "arch": "amd64"}, {"platform": "macos", "arch": "arm64"}
    )
    assert derive_labels(r) == {"os": "macos", "arch": "universal", "libc": None}


def test_mixed_platforms_masked() -> None:
    r = row(
        {"platform": "linux", "arch": "amd64"}, {"platform": "windows", "arch": "amd64"}
    )
    assert derive_labels(r) == {"os": None, "arch": None, "libc": None}


def test_linux_libc_and_mixed_arch() -> None:
    r = row(
        {"platform": "linux", "arch": "amd64", "libc": "musl"},
        {"platform": "linux", "arch": "arm64", "libc": "musl"},
    )
    assert derive_labels(r) == {"os": "linux", "arch": None, "libc": "musl"}


def test_libc_only_for_linux_and_unknown_os() -> None:
    assert (
        derive_labels(row({"platform": "windows", "arch": "386", "libc": "gnu"}))[
            "libc"
        ]
        is None
    )
    assert derive_labels(row({"platform": "", "arch": "arm64"}))["os"] is None


def test_conflicts_and_uninspected_masked() -> None:
    target = {"platform": "linux", "arch": "amd64"}
    assert derive_labels(row(target, conflicts=["x"]))["os"] is None
    assert derive_labels(row(target, status="failed"))["os"] is None


def test_raw_inspector_values_fold_into_served_classes() -> None:
    assert derive_labels(row({"platform": "freebsd", "arch": "elf:8"})) == {
        "os": "other",
        "arch": "other",
        "libc": None,
    }
    assert derive_labels(row({"platform": "musl", "arch": "linux-armhf"})) == {
        "os": "linux",
        "arch": "arm",
        "libc": "musl",
    }


def test_windows_installer_stub_arch_masked() -> None:
    r = row({"platform": "windows", "arch": "386"}, name="App_1.0_x64-setup.exe")
    assert derive_labels(r) == {"os": "windows", "arch": None, "libc": None}
