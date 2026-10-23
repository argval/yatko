/** Shared frontend → Go backend connection settings. */
export const BACKEND_URL = process.env.BACKEND_URL || "http://localhost:8080";

/** Cap wall time so hung upstreams cannot pin a Fluid instance near max duration. */
export const BACKEND_FETCH_TIMEOUT_MS = 8_000;

/** The release lookup is the one call the page cannot render without. A cold
 *  backend container can take ~5s to boot before handling it, so allow more. */
export const BACKEND_RELEASE_TIMEOUT_MS = 15_000;

/** How long Next may reuse a cached backend/GitHub response before revalidating. */
export const BACKEND_FETCH_REVALIDATE_SECONDS = 3600;
