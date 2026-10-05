"""Compatibility launcher; Git should call xmlmerge.exe git-driver directly."""
from pathlib import Path
import subprocess
import sys


def main():
    binary_dir = Path(__file__).resolve().parent.parent / "bin"
    command = [str(binary_dir / "xmlmerge.exe"), "git-driver",
               "--rules", str(binary_dir / "rules.xml"),
               "--policy", str(binary_dir / "merge-policy.xml"), *sys.argv[1:]]
    try:
        code = subprocess.run(command, check=False).returncode
        return code if code >= 0 else 2
    except OSError as error:
        print(f"XML Merge: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
