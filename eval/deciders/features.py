"""Build what a router sees at the start of each task: the prompt and the visible spec.

Usage: python3 -I features.py <workdir> > features.jsonl

One JSON line per valid task in <workdir>/suite.tsv: task, prompt (word for word
what run_cell.sh sends), packages, test_files, test_lines. Nothing from the answer
diff and nothing from the matrix is included.
"""
import csv
import json
import subprocess
import sys

PROMPT = """You are in a Go repository. Implement this change:

{message}

These test files describe the required behavior. They must pass, and you must not edit them:
{tests}
Work only in this directory. Keep existing behavior unless the change requires otherwise.
Before you finish, run go build ./... and go test on the packages above. The network is off;
every dependency you need is already in the module cache.
Do not ask questions; make reasonable choices. When the change is done, reply DONE."""


def git(repo, *args):
    return subprocess.run(["git", "-C", repo, *args], check=True, capture_output=True, text=True).stdout


def task_features(row):
    repo, commit, parent = row["repo"], row["commit"], row["parent"]
    spec = [p for p in git(repo, "diff", "--name-only", "--diff-filter=AM", parent, commit).split()
            if p.endswith("_test.go") or "/testdata/" in "/" + p]
    tests = [p for p in spec if p.endswith("_test.go")]
    lines = sum(git(repo, "show", f"{commit}:{p}").count("\n") for p in tests)
    message = git(repo, "log", "-1", "--format=%B", commit).rstrip("\n")
    # run_cell.sh builds the list with printf '  %s\n', then command substitution drops the last newline.
    listing = "\n".join(f"  {t}" for t in tests)
    return {"task": row["id"], "prompt": PROMPT.format(message=message, tests=listing),
            "packages": len(row["packages"].split()), "test_files": len(tests), "test_lines": lines}


def main():
    with open(f"{sys.argv[1]}/suite.tsv") as f:
        for row in csv.DictReader(f, delimiter="\t"):
            if row["verdict"] == "valid":
                print(json.dumps(task_features(row)))


if __name__ == "__main__":
    main()
