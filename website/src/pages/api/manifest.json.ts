import type { APIRoute } from 'astro';
import { dockerImage, applianceImage, tools } from '../../data/manifest';
import { getLatestRelease, deriveArtifact, strV } from '../../lib/github';

export const GET: APIRoute = async () => {
  const release = await getLatestRelease();
  const version = strV(release?.tag_name) ?? null;
  const ghArtifacts = (release?.assets ?? []).map(deriveArtifact);

  const body = {
    schema: 'bootimus.manifest/v1',
    generated: new Date().toISOString(),
    release: {
      version,
      tag: release?.tag_name ?? null,
      released: release?.published_at ?? null,
      url: release?.html_url ?? null,
      prerelease: release?.prerelease ?? false,
      source: release ? 'github' : 'none',
      artifacts: [
        dockerImage,
        ...ghArtifacts,
        applianceImage,
      ],
    },
    tools,
  };

  return new Response(JSON.stringify(body, null, 2), {
    headers: {
      'Content-Type': 'application/json; charset=utf-8',
      'Cache-Control': 'public, max-age=60, s-maxage=300',
      'Access-Control-Allow-Origin': '*',
    },
  });
};
