#!/usr/bin/env python3
"""PowerShell menu tests in a real terminal: Windows ConPTY or POSIX PTY."""
import os
from pathlib import Path
import queue
import shutil
import subprocess
import tempfile
import threading
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
BIN = ROOT / ("cdt.exe" if os.name == "nt" else "cdt")


class WindowsTerminal:
    def __init__(self, args, env):
        import ctypes as c
        from ctypes import wintypes as w
        self.c = c
        self.k = c.WinDLL("kernel32", use_last_error=True)

        class Coord(c.Structure):
            _fields_ = [("X", c.c_short), ("Y", c.c_short)]

        class Startup(c.Structure):
            _fields_ = [
                ("cb", w.DWORD), ("reserved", w.LPWSTR), ("desktop", w.LPWSTR),
                ("title", w.LPWSTR), ("x", w.DWORD), ("y", w.DWORD),
                ("width", w.DWORD), ("height", w.DWORD), ("xchars", w.DWORD),
                ("ychars", w.DWORD), ("fill", w.DWORD), ("flags", w.DWORD),
                ("show", w.WORD), ("reserved2size", w.WORD),
                ("reserved2", c.POINTER(c.c_byte)), ("stdin", w.HANDLE),
                ("stdout", w.HANDLE), ("stderr", w.HANDLE),
            ]

        class StartupEx(c.Structure):
            _fields_ = [("startup", Startup), ("attributes", c.c_void_p)]

        class ProcessInfo(c.Structure):
            _fields_ = [("process", w.HANDLE), ("thread", w.HANDLE),
                        ("pid", w.DWORD), ("tid", w.DWORD)]

        signatures = {
            "CreatePipe": ([c.POINTER(w.HANDLE), c.POINTER(w.HANDLE), c.c_void_p, w.DWORD], w.BOOL),
            "CreatePseudoConsole": ([Coord, w.HANDLE, w.HANDLE, w.DWORD, c.POINTER(w.HANDLE)], c.c_long),
            "InitializeProcThreadAttributeList": ([c.c_void_p, w.DWORD, w.DWORD, c.POINTER(c.c_size_t)], w.BOOL),
            "UpdateProcThreadAttribute": ([c.c_void_p, w.DWORD, c.c_size_t, c.c_void_p, c.c_size_t, c.c_void_p, c.c_void_p], w.BOOL),
            "DeleteProcThreadAttributeList": ([c.c_void_p], None),
            "CreateProcessW": ([w.LPCWSTR, w.LPWSTR, c.c_void_p, c.c_void_p, w.BOOL, w.DWORD,
                                c.c_void_p, w.LPCWSTR, c.POINTER(StartupEx), c.POINTER(ProcessInfo)], w.BOOL),
            "ReadFile": ([w.HANDLE, c.c_void_p, w.DWORD, c.POINTER(w.DWORD), c.c_void_p], w.BOOL),
            "WriteFile": ([w.HANDLE, c.c_void_p, w.DWORD, c.POINTER(w.DWORD), c.c_void_p], w.BOOL),
            "CloseHandle": ([w.HANDLE], w.BOOL),
            "ClosePseudoConsole": ([w.HANDLE], None),
            "TerminateProcess": ([w.HANDLE, w.UINT], w.BOOL),
            "WaitForSingleObject": ([w.HANDLE, w.DWORD], w.DWORD),
        }
        for name, (arguments, result) in signatures.items():
            function = getattr(self.k, name)
            function.argtypes, function.restype = arguments, result
        input_read, self.input = w.HANDLE(), w.HANDLE()
        self.output, output_write = w.HANDLE(), w.HANDLE()
        self.check(self.k.CreatePipe(c.byref(input_read), c.byref(self.input), None, 0))
        self.check(self.k.CreatePipe(c.byref(self.output), c.byref(output_write), None, 0))
        self.console = w.HANDLE()
        result = self.k.CreatePseudoConsole(Coord(180, 40), input_read, output_write, 0, c.byref(self.console))
        self.k.CloseHandle(input_read)
        self.k.CloseHandle(output_write)
        if result != 0:
            raise OSError(f"CreatePseudoConsole failed: {result:#x}")
        size = c.c_size_t()
        self.k.InitializeProcThreadAttributeList(None, 1, 0, c.byref(size))
        attributes = c.create_string_buffer(size.value)
        self.check(self.k.InitializeProcThreadAttributeList(attributes, 1, 0, c.byref(size)))
        self.check(self.k.UpdateProcThreadAttribute(attributes, 0, 0x00020016,
                                                    self.console, c.sizeof(w.HANDLE), None, None))
        startup = StartupEx()
        startup.startup.cb = c.sizeof(startup)
        startup.attributes = c.cast(attributes, c.c_void_p)
        info = ProcessInfo()
        environment = c.create_unicode_buffer("\0".join(f"{k}={v}" for k, v in sorted(env.items())) + "\0\0")
        command = c.create_unicode_buffer(subprocess.list2cmdline(args))
        try:
            self.check(self.k.CreateProcessW(None, command, None, None, False, 0x00080000 | 0x00000400,
                                             environment, str(ROOT), c.byref(startup), c.byref(info)))
        finally:
            self.k.DeleteProcThreadAttributeList(attributes)
        self.process = info.process
        self.k.CloseHandle(info.thread)

    def check(self, result):
        if not result:
            raise self.c.WinError(self.c.get_last_error())

    def read(self):
        from ctypes import wintypes as w
        data = self.c.create_string_buffer(65536)
        count = w.DWORD()
        if not self.k.ReadFile(self.output, data, len(data), self.c.byref(count), None):
            return b""
        return data.raw[:count.value]

    def send(self, data):
        from ctypes import wintypes as w
        count = w.DWORD()
        self.check(self.k.WriteFile(self.input, data, len(data), self.c.byref(count), None))

    def close(self):
        self.k.TerminateProcess(self.process, 0)
        self.k.WaitForSingleObject(self.process, 5000)
        self.k.CloseHandle(self.input)
        self.k.ClosePseudoConsole(self.console)
        self.k.CloseHandle(self.output)
        self.k.CloseHandle(self.process)


class PosixTerminal:
    def __init__(self, args, env):
        import fcntl
        import pty
        import struct
        import termios
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            os.execve(args[0], args, env)
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 180, 0, 0))

    def read(self):
        try:
            return os.read(self.fd, 65536)
        except OSError:
            return b""

    def send(self, data):
        os.write(self.fd, data)

    def close(self):
        import signal
        os.kill(self.pid, signal.SIGKILL)
        os.waitpid(self.pid, 0)
        os.close(self.fd)


class Shell:
    def __init__(self, env):
        executable = shutil.which("pwsh", path=env["PATH"])
        script = ("function global:prompt { 'CDT_PROMPT> ' }; "
                  "cdt init powershell | Out-String | Invoke-Expression")
        args = [executable, "-NoLogo", "-NoProfile", "-NoExit", "-Command", script]
        self.terminal = (WindowsTerminal if os.name == "nt" else PosixTerminal)(args, env)
        self.chunks = queue.Queue()
        self.data = b""
        self.reader = threading.Thread(target=self.read, daemon=True)
        self.reader.start()
        try:
            self.until(b"CDT_PROMPT> ", timeout=40)
        except BaseException:
            self.close()
            raise

    def read(self):
        tail = b""
        while True:
            chunk = self.terminal.read()
            if not chunk:
                self.chunks.put(None)
                return
            # PowerShell asks the terminal for its cursor position.
            if b"\x1b[6n" in tail + chunk:
                self.terminal.send(b"\x1b[1;1R")
            tail = (tail + chunk)[-3:]
            self.chunks.put(chunk)

    def send(self, text):
        self.terminal.send(text.encode() if isinstance(text, str) else text)

    def until(self, needle, timeout=15):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if needle in self.data:
                result, self.data = self.data.split(needle, 1)
                return result
            try:
                chunk = self.chunks.get(timeout=0.1)
            except queue.Empty:
                continue
            if chunk is None:
                break
            self.data += chunk
        raise AssertionError(f"terminal did not emit {needle!r}: {self.data!r}")

    def command(self, text):
        self.send(text + "\r")
        self.send("Write-Output ('CDT_DONE' + '_' + $global:LASTEXITCODE)\r")
        out = self.until(b"CDT_DONE_")
        status = self.until(b"\n").strip()
        self.until(b"CDT_PROMPT> ")
        return out, status

    def close(self):
        self.terminal.close()
        self.reader.join(timeout=2)


class PowerShellTerminalTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve()
        self.env = dict(os.environ, XDG_CONFIG_HOME=str(self.root / "config"),
                        PATH=str(ROOT) + os.pathsep + os.environ["PATH"], TERM="xterm-256color")
        self.a = self.root / "a space ' [literal] $(INJECTED)"
        self.b = self.root / "b directory"
        for path, tag in [(self.a, "alpha"), (self.b, "beta")]:
            path.mkdir()
            subprocess.run([str(BIN), "-ap", str(path), "api", tag], env=self.env,
                           capture_output=True, check=True, timeout=20)
        self.shell = Shell(self.env)
        self.addCleanup(self.shell.close)

    def menu(self):
        self.shell.send("cdt api\r")
        self.shell.until(b"cdt> 1/2")

    def location(self, expected):
        out, code = self.shell.command("Write-Output ('HERE:' + (Get-Location).ProviderPath)")
        self.assertIn(("HERE:" + str(expected)).encode(), out)
        return code

    def test_tab_and_enter(self):
        self.menu()
        self.shell.send(b"\t\r")
        out = self.shell.until(b"CDT_PROMPT> ")
        self.assertIn(b"cdt: entered ", out)
        self.assertEqual(self.location(self.b), b"0")

    def test_arrows(self):
        self.menu()
        self.shell.send(b"\x1b[B\x1b[A\r")
        self.shell.until(b"CDT_PROMPT> ")
        self.assertEqual(self.location(self.a), b"0")

    def test_cancel_and_restore(self):
        self.shell.command("cdt alpha")
        for key in [b"\x1b", b"\x07", b"\x03"]:
            with self.subTest(key=key):
                self.menu()
                self.shell.send(key)
                out = self.shell.until(b"CDT_PROMPT> ")
                self.assertIn(b"cdt: cancelled; directory unchanged", out)
                self.assertEqual(self.location(self.a), b"130")
                out, _ = self.shell.command("Write-Output ('CTRL:' + [Console]::TreatControlCAsInput)")
                self.assertIn(b"CTRL:False", out)


if __name__ == "__main__":
    unittest.main()
