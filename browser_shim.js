// Browser shim for the Wails JS bindings: every generated binding calls
// window.go.main.App.<Method>(...args) and expects a promise that resolves
// with the Go result or rejects with the Go error. This proxy routes those
// calls to the /rpc/ HTTP endpoint of browser_main.go.
(() => {
	const call = (name) => (...args) =>
		fetch("/rpc/" + name, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(args),
		}).then(async (res) => {
			if (!res.ok) throw new Error(await res.text());
			const t = await res.text();
			return t.length ? JSON.parse(t) : null;
		});

	window.go = { main: { App: new Proxy({}, { get: (_, name) => call(name) }) } };
})();
