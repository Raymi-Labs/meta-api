// A completed settings write invalidates status reads started before it.
// Both public and console layouts share this guard with the enterprise switch.
let revision = 0;

export function invalidateSiteStatusReads() {
  revision++;
}

export async function readCurrentSiteStatus(read) {
  const started = revision;
  const result = await read();
  return started === revision ? result : null;
}
