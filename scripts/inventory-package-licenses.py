#!/usr/bin/env python3
"""Record declared NuGet licence metadata after restore; not a licence approval."""
import json
from pathlib import Path
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]


def main():
    packages = {}
    for repository in ('packet.net', 'pdn-net'):
        for assets in (ROOT / 'upstream' / repository).glob('**/obj/project.assets.json'):
            data = json.loads(assets.read_text())
            for key, library in data.get('libraries', {}).items():
                if library['type'] != 'package':
                    continue
                entry = packages.setdefault(key, {'projects': set()})
                entry['projects'].add(str(assets.parent.parent.relative_to(ROOT)))
                for folder in data.get('packageFolders', {}):
                    for nuspec in (Path(folder) / library['path']).glob('*.nuspec'):
                        for element in ET.parse(nuspec).iter():
                            name = element.tag.rsplit('}', 1)[-1]
                            if name in ('license', 'licenseUrl', 'copyright', 'repository'):
                                entry[name] = {'text': element.text, 'attributes': element.attrib}
                        entry['nuspec'] = str(nuspec)
    for entry in packages.values():
        entry['projects'] = sorted(entry['projects'])
    output = ROOT / 'artifacts/m0/package-license-metadata.json'
    output.write_text(json.dumps(packages, indent=2) + '\n')
    print(f'{len(packages)} Paketidentitäten; deklarierte Metadaten, keine Lizenzfreigabe')
    print('Ohne license-Element:', ', '.join(k for k, v in packages.items() if 'license' not in v))
    return 0 if packages else 1


if __name__ == '__main__':
    raise SystemExit(main())
