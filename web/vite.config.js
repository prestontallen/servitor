import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Build output lands directly in the Go embed directory. `go:embed static`
// picks it up; emptyOutDir clears the old vanilla-JS GUI on each build.
export default defineConfig({
	plugins: [svelte()],
	build: {
		outDir: '../internal/api/static',
		emptyOutDir: true,
		target: 'es2022'
	},
	server: {
		proxy: {
			// SERVITOR_API=http://host:port points the dev GUI at another daemon
			'/api': process.env.SERVITOR_API || 'http://localhost:8181'
		}
	}
});
