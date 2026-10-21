#!/usr/bin/env python3
"""Real zsh PTY tests; uses only the Python standard library."""
import fcntl
import os
from pathlib import Path
import pty
import select
import signal
import shutil
import struct
import subprocess
import tempfile
import termios
import time
import unittest


ROOT = Path(__file__).resolve().parents[1]
BIN = ROOT / "cdt"


class Shell:
    def __init__(self, env):
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            os.execve(shutil.which("zsh"), ["zsh", "-f", "-i"], env)
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 180, 0, 0))
        self.data = b""
        self.send("PS1='CDT_PROMPT> '; PS2=''; RPS1=''; unsetopt beep; bindkey -e\n")
        self.until(b"CDT_PROMPT> ")
        self.command('autoload -Uz compinit; compinit -D; eval "$(command cdt init zsh)"')

    def send(self, text):
        os.write(self.fd, text.encode() if isinstance(text, str) else text)

    def until(self, needle, timeout=10):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if needle in self.data:
                result, self.data = self.data.split(needle, 1)
                return result
            ready, _, _ = select.select([self.fd], [], [], 0.1)
            if ready:
                try:
                    self.data += os.read(self.fd, 65536)
                except OSError:
                    break
        raise AssertionError(f"timeout waiting for {needle!r}: {self.data!r}")

    def command(self, text):
        self.data = b""
        self.send(text + "\n")
        # A marker split across shell words cannot be confused with input echo.
        self.send("print -r -- CDT_DONE'_'$?\n")
        result = self.until(b"CDT_DONE_")
        status = self.until(b"\n").strip()
        self.until(b"CDT_PROMPT> ")
        return result, status

    def close(self):
        try:
            os.kill(self.pid, signal.SIGKILL)
            os.waitpid(self.pid, 0)
        finally:
            os.close(self.fd)


class Integration(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name).resolve()
        self.env = dict(os.environ, XDG_CONFIG_HOME=str(self.root / "config"),
                        PATH=str(ROOT) + os.pathsep + os.environ["PATH"],
                        TERM="xterm-256color", ZDOTDIR=str(self.root))
        self.a = self.root / "a space ' $(touch PWNED)"
        self.b = self.root / "b 中文"
        self.a.mkdir()
        self.b.mkdir()
        self.run_cdt("-ap", str(self.a), "api", "alpha")
        self.run_cdt("-ap", str(self.b), "api", "beta")
        self.shell = Shell(self.env)

    def run_cdt(self, *args):
        return subprocess.run([str(BIN), *args], env=self.env, check=True,
                              capture_output=True, text=True)

    def tearDown(self):
        self.shell.close()
        self.tmp.cleanup()

    def test_unique_and_noninteractive(self):
        out, code = self.shell.command("cdt alpha; print -r -- HERE:$PWD")
        self.assertEqual(code, b"0", out)
        self.assertIn(("HERE:" + str(self.a)).encode(), out)
        self.assertIn(b"cdt: entered ", out)
        self.assertFalse((ROOT / "PWNED").exists())
        script = 'eval "$(command cdt init zsh)"; before=$PWD; cdt api; rc=$?; [[ $PWD == $before ]]; print RESULT:$rc:$?'
        result = subprocess.run(["zsh", "-fc", script], env=self.env, capture_output=True)
        self.assertIn(b"RESULT:2:0", result.stdout)
        self.assertIn(b"ambiguous", result.stderr)
        result = subprocess.run(["zsh", "-fc", 'eval "$(command cdt init zsh)"; cdt alpha'],
                                env=self.env, capture_output=True)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, b"")
        self.assertEqual(result.stderr, b"")

    def test_mutation_feedback(self):
        self.shell.command("cdt alpha")
        out, code = self.shell.command("cdt -a api new new")
        self.assertEqual(code, b"0", out)
        for text in [b"Added: [new]", b"Already present (skipped): [api]",
                     b"Repeated input (ignored): [new]",
                     b"Previous (own): [alpha, api]", b"Current (own): [alpha, api, new]"]:
            self.assertIn(text, out)
        out, code = self.shell.command("cdt -a new")
        self.assertEqual(code, b"0", out)
        self.assertIn(b"unchanged", out)
        out, _ = self.shell.command("cdt -r new missing")
        self.assertIn(b"Removed: [new]", out)
        self.assertIn(b"Not set here (skipped): [missing]", out)
        out, _ = self.shell.command("cdt -d")
        self.assertIn(b"unregistered", out)
        self.assertTrue(self.a.is_dir())

    def test_menu_accept(self):
        self.shell.command("zmodload zsh/complist; bindkey -M menuselect '^M' accept-line; bindkey -M menuselect '^G' send-break")
        self.shell.data = b""
        self.shell.send("cdt api\n")
        menu = self.shell.until(b"[api, beta]")
        self.assertIn(b"[alpha, api]", menu)
        self.shell.send(b"\t\r")
        self.shell.until(b"CDT_PROMPT> ")
        out, code = self.shell.command("print -r -- HERE:$PWD")
        self.assertEqual(code, b"0")
        self.assertIn(("HERE:" + str(self.b)).encode(), out)
        out, _ = self.shell.command("bindkey -M menuselect '^M'; bindkey -M menuselect '^G'")
        self.assertIn(b'"^M" accept-line', out)
        self.assertIn(b'"^G" send-break', out)

    def test_menu_cancel(self):
        self.shell.command("builtin cd /")
        self.shell.data = b""
        self.shell.send("cdt api\n")
        self.shell.until(b"[api, beta]")
        self.shell.send(b"\x07")
        cancelled = self.shell.until(b"CDT_PROMPT> ")
        self.assertIn(b"cdt: cancelled; directory unchanged", cancelled)
        out, _ = self.shell.command("print -r -- HERE:$PWD")
        self.assertIn(b"HERE:/", out)

    def test_completion(self):
        self.shell.data = b""
        self.shell.send("cdt alph\t")
        time.sleep(0.3)
        self.shell.send(b"\r")
        self.shell.until(b"CDT_PROMPT> ")
        out, _ = self.shell.command("print -r -- HERE:$PWD")
        self.assertIn(("HERE:" + str(self.a)).encode(), out)

    def test_arrows_and_ctrl_c(self):
        self.shell.data = b""
        self.shell.send("cdt api\n")
        self.shell.until(b"[api, beta]")
        self.shell.send(b"\x1b[B\r")
        self.shell.until(b"CDT_PROMPT> ")
        out, _ = self.shell.command("print -r -- HERE:$PWD")
        self.assertIn(("HERE:" + str(self.b)).encode(), out)
        self.shell.data = b""
        self.shell.send("cdt api\n")
        self.shell.until(b"[api, beta]")
        self.shell.send(b"\x03")
        cancelled = self.shell.until(b"CDT_PROMPT> ")
        self.assertIn(b"cdt: cancelled; directory unchanged", cancelled)
        out, _ = self.shell.command("print -r -- HERE:$PWD")
        self.assertIn(("HERE:" + str(self.b)).encode(), out)

    def test_inheritance_completion(self):
        self.run_cdt("-ap", str(self.a), "scoped")
        # Only a is under this registered parent; the other scope stays separate.
        self.run_cdt("-ap", str(self.root), "work")
        self.shell.data = b""
        self.shell.send("cdt work alph\t")
        time.sleep(0.3)
        self.shell.send(b"\r")
        self.shell.until(b"CDT_PROMPT> ")
        out, _ = self.shell.command("print -r -- HERE:$PWD")
        self.assertIn(("HERE:" + str(self.a)).encode(), out)
        out, _ = self.shell.command("cdt work; print -r -- HERE:$PWD")
        self.assertIn(("HERE:" + str(self.root)).encode(), out)

    def test_path_completion(self):
        target = self.root / "target-completion"
        target.mkdir()
        self.shell.command("builtin cd " + str(self.root))
        self.shell.data = b""
        self.shell.send("cdt -ap target-comp\t")
        time.sleep(0.3)
        self.shell.send(b" added\r")
        self.shell.until(b"CDT_PROMPT> ")
        out, _ = self.shell.command("cdt added; print -r -- HERE:$PWD")
        self.assertIn(("HERE:" + str(target)).encode(), out)

    def test_edit_in_vi(self):
        self.shell.command("export VISUAL=; export EDITOR='vi -u NONE -N -i NONE'")
        self.shell.data = b""
        self.shell.send("cdt -e\n")
        self.shell.until(b".edit-")
        self.shell.send(b"\x1b:%s/alpha/edited/g\r:wq\r")
        self.shell.until(b"CDT_PROMPT> ")
        out, code = self.shell.command("cdt edited; print -r -- HERE:$PWD")
        self.assertEqual(code, b"0", out)
        self.assertIn(("HERE:" + str(self.a)).encode(), out)


if __name__ == "__main__":
    unittest.main()
