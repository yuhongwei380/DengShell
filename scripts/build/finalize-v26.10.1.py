#!/usr/bin/env python3
"""One-time, user-authorized retirement of the interim v26.10.1 build releases.

The unified v26.10.1 stable release (tag v26.10.1-build26102401, ten-digit
build number) supersedes every interim v26.10.1-build2026... publication.
This script deletes those interim GitHub releases and tags after both
platform publication jobs succeed. Never reads local Git credentials; gh
uses the workflow's existing, ephemeral contents-write token.
"""
import json
import os
from pathlib import Path
import subprocess

REPOSITORY = 'yuhongwei380/DevShell'
STABLE_TAG = 'v26.10.1-build26102401'
RETIRED_TAGS = (
    'v26.10.1-build20261023150',
    'v26.10.1-build20261024152',
    'v26.10.1-build20261024153',
    'v26.10.1-build20261024154',
    'v26.10.1-build20261024155',
    'v26.10.1-build20261024156',
)
REQUIRED_ASSETS = {
    'DevShell-Setup-x64.exe', 'up.exe', 'up.exe.json',
    'DevShell-macos-universal.dmg', 'DevShell-macos-universal.zip',
    'up-macos-arm64.json', 'up-macos-amd64.json',
    'SHA256SUMS.txt', 'build-info.json',
}


def gh(*args):
    result = subprocess.run(['gh', *args], text=True, capture_output=True, check=True)
    return result.stdout


def api(path):
    return json.loads(gh('api', f'repos/{REPOSITORY}/{path}'))


def releases():
    pages = json.loads(gh('api', '--paginate', '--slurp',
                          f'repos/{REPOSITORY}/releases?per_page=100'))
    return [release for page in pages for release in page]


def validate(stable, inventory):
    if (stable.get('tagName') != STABLE_TAG or stable.get('isDraft') is not False
            or stable.get('isPrerelease') is not False):
        raise ValueError('Expected the published v26.10.1 stable release')
    assets = {asset['name']: asset for asset in stable.get('assets', [])}
    if not REQUIRED_ASSETS.issubset(assets):
        raise ValueError(f'Missing stable assets: {sorted(REQUIRED_ASSETS - assets.keys())}')
    if STABLE_TAG in RETIRED_TAGS:
        raise ValueError('Refusing to retire the stable release')
    # Validate the entire deletion scope before changing any release or tag.
    for release in inventory:
        if release.get('tag_name') in RETIRED_TAGS and release.get('draft') is not False:
            raise ValueError(f'Refusing to remove a draft release: {release["tag_name"]}')


def main():
    if os.environ.get('GH_REPO') != REPOSITORY:
        raise ValueError('Unexpected repository; no changes made')
    stable = json.loads(gh('release', 'view', STABLE_TAG, '--json',
                           'tagName,isDraft,isPrerelease,assets'))
    inventory = releases()
    validate(stable, inventory)
    gh('release', 'edit', STABLE_TAG, '--latest')
    present = {release['tag_name'] for release in inventory}
    for tag in RETIRED_TAGS:
        if tag in present:
            gh('release', 'delete', tag, '--yes')
        # Separate deletion also makes a rerun recover from a partial cleanup.
        refs = api(f'git/matching-refs/tags/{tag}')
        if any(ref.get('ref') == f'refs/tags/{tag}' for ref in refs):
            gh('api', '--method', 'DELETE', f'repos/{REPOSITORY}/git/refs/tags/{tag}')
        print(f'Retired {tag}', flush=True)
    remaining = {release['tag_name'] for release in releases()}
    leftover = [tag for tag in RETIRED_TAGS if tag in remaining]
    for tag in RETIRED_TAGS:
        refs = api(f'git/matching-refs/tags/{tag}')
        if any(ref.get('ref') == f'refs/tags/{tag}' for ref in refs):
            leftover.append(tag)
    if leftover:
        raise ValueError(f'Interim build cleanup verification failed: {leftover}')
    if api('releases/latest').get('tag_name') != STABLE_TAG:
        raise ValueError('Stable release is not Latest')
    summary = os.environ.get('GITHUB_STEP_SUMMARY')
    if summary:
        with Path(summary).open('a', encoding='utf-8') as output:
            output.write(f'Published **{STABLE_TAG}** with all platform assets.\n\n')
            output.write('Removed interim build releases and tags:\n\n'
                         + ''.join(f'- {tag}\n' for tag in RETIRED_TAGS))


if __name__ == '__main__':
    main()
