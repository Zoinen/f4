"""Validate native Windows foreground evidence for a scrolling capture.

Qt's active-window state alone can stay true while another process owns the
Windows foreground window. This gate requires the independent native check
at admission, folder navigation and every recorded input event.
"""

import argparse
import json
from pathlib import Path


def verify(packet, navigation, admission, identity):
    wheel = packet.get('wheel', [])
    observer = admission.get('observer', {})
    checks = {
        'ownedForegroundAtAdmission': admission.get('foregroundPid')
        == identity.get('qtPid') and isinstance(identity.get('qtPid'), int),
        'observerAbsentAtAdmission': observer.get('cleanDesktopObserverCheck') is True
        and observer.get('observerProcesses') == [],
        'nativeForegroundAtNavigation': navigation.get('nativeForeground') is True,
        'hasInputSamples': bool(wheel),
        'nativeForegroundForAllInput': bool(wheel)
        and all(row.get('nativeForeground') is True for row in wheel),
    }
    return {
        'acceptedNativeForegroundScope': all(checks.values()),
        'checks': checks,
        'inputSamples': len(wheel),
        'inputSamplesWithoutNativeForeground': sum(
            row.get('nativeForeground') is not True for row in wheel),
        'scope': 'Foreground admission and sampled input; not continuous foreground monitoring',
        'physicalMotionVerified': False,
        'fullPerformanceAccepted': False,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('capture', type=Path)
    parser.add_argument('case')
    args = parser.parse_args()
    def read(name):
        return json.loads((args.capture / name).read_text())
    result = verify(read(args.case + '.json'), read('entry-navigation.json'),
                    read('foreground-admission.json'), read('identity.json'))
    (args.capture / 'native-foreground-admission.json').write_text(json.dumps(result, indent=2))
    print(json.dumps(result, indent=2))
    if not result['acceptedNativeForegroundScope']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
