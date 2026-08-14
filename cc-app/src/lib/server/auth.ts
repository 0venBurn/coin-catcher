import { betterAuth } from 'better-auth';
import { drizzleAdapter } from 'better-auth/adapters/drizzle';
import { sveltekitCookies } from 'better-auth/svelte-kit';
import { getRequestEvent } from '$app/server';
import { db } from './db';
import * as schema from './db/schema';

export const auth = betterAuth({
	database: drizzleAdapter(db, {
		provider: 'pg',
		schema
	}),
	emailAndPassword: {
		enabled: true
	},
	// Propagates Better Auth session cookies onto SvelteKit server responses
	// when auth.api.* is called from load functions / form actions.
	plugins: [sveltekitCookies(getRequestEvent)]
});
