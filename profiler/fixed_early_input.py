"""Validate a fixed early-input capture without hiding late or missing slots."""
from collections import Counter
from pathlib import Path

from requested_row_timeline import distribution

DELAY_NS = 200_000_000
PERIOD_NS = 10_000_000
DURATION_NS = 30_000_000_000
COUNT = 3000
DIRECTION_SLOTS = 150


def verify(packet, startup, navigation):
    def require(condition, message):
        if not condition:
            raise ValueError(message)
    def path(value):
        return value.replace('\\', '/').rstrip('/').casefold()

    require(startup['emptyAtStartup'] and startup['configuredBeforeQml'],
            'Cache was not configured empty before QML')
    require(Path(packet['coldCacheRoot']).resolve() == Path(startup['cacheRoot']).resolve(),
            'Cache identity changed')
    require(navigation['initializedBeforeNavigation'] and navigation['bothPanels']
            and navigation['action'] == 'panel.navigatePath', 'Wrong folder-entry action')
    require(navigation['pid'] == startup['pid'], 'Navigation process identity changed')
    require(navigation['renderedFramesBeforeNavigation'] >= 3
            and navigation['navigationSentNs']-navigation['firstObservedSwapNs'] >= 2e9,
            'Application was not initialized before navigation')
    require(set(navigation['sourcePaths']) == {'0', '1'}
            and all(path(p) == path(navigation['from'])
                    for p in navigation['sourcePaths'].values()), 'Wrong initial folders')
    require(all(navigation['cacheBeforeNavigation'][k] == 0
                for k in ('frames', 'stores', 'pending')), 'Navigation cache was not empty')
    require(path(navigation['target']) == 'c:/users/xs/downloads', 'Wrong target folder')
    require(path(navigation['from']) != path(navigation['target']), 'No actual folder entry')
    require(navigation['active'] and navigation['nativeForeground']
            and navigation['dpr'] == 1.75 and abs(navigation['hz']-160) <= 1,
            'Navigation display/focus mismatch')
    require(not packet['allEntryWarmTourVerified'] and not packet['warmTour']
            and not packet['middle'] and packet['coldPositioned'], 'Wrong workload')

    spec = packet['fixedSchedule']
    for key, expected in [('startDelayNs', DELAY_NS), ('periodNs', PERIOD_NS),
                          ('durationNs', DURATION_NS), ('count', COUNT),
                          ('directionSlots', DIRECTION_SLOTS)]:
        require(spec[key] == expected, 'Fixed schedule specification changed: '+key)
    require(spec['readinessTriggered'] is False and spec['catchUpBursts'] is False,
            'Readiness-driven input or catch-up replay is not this workload')
    start = packet['startNs']
    require(start-navigation['navigationSentNs'] == DELAY_NS,
            'Input start was shifted from the navigation deadline')
    require(spec['armedNs'] >= navigation['navigationSentNs'], 'Armed before navigation')
    require(spec['inputFinishedNs'] >= start+DURATION_NS
            and packet['endNs'] >= spec['inputFinishedNs'], 'Truncated input window')

    inputs = packet['wheel']
    require(bool(inputs), 'No input was delivered')
    delivered, previous, lateness = set(), -1, []
    for row in inputs:
        slot = row['slot']
        require(type(slot) is int and previous < slot < COUNT, 'Invalid or reordered slot')
        require(row['scheduledNs'] == start+slot*PERIOD_NS, 'Slot deadline changed')
        require(row['scheduledNs'] <= row['timeNs'] < start+DURATION_NS,
                'Input outside its planned window')
        require(row['latenessNs'] == row['timeNs']-row['scheduledNs'], 'Wrong lateness')
        require(row['up'] == ((slot//DIRECTION_SLOTS) % 2 == 1), 'Position-driven direction')
        require(row['active'] and row['nativeForeground'] and row['dpr'] == 1.75
                and abs(row['hz']-160) <= 1 and row['density'] == 152
                and row['mode'] == 'masonry', 'Input display/focus/mode changed')
        require(type(row['entries']) is int and 0 <= row['entries'] <= 192,
                'Dataset exceeds the admitted folder scope')
        require(path(row['path']) in {path(navigation['from']), path(navigation['target'])},
                'Input was routed to an unrelated folder')
        delivered.add(slot)
        previous = slot
        lateness.append(row['latenessNs']/1e6)
    require(spec['firstDispatchNs'] == inputs[0]['timeNs'], 'First dispatch marker changed')
    require(inputs[-1]['entries'] == 192
            and path(inputs[-1]['path']) == path(navigation['target']),
            'The requested dataset never finished loading')

    skipped = set()
    for gap in spec['skippedSlots']:
        first, count = gap['firstSlot'], gap['count']
        require(type(first) is int and type(count) is int and count > 0
                and 0 <= first < first+count <= COUNT, 'Invalid skipped range')
        require(gap['observedNs'] >= start+(first+count-1)*PERIOD_NS,
                'Future slots were skipped before their deadlines')
        for slot in range(first, first+count):
            require(slot not in skipped and slot not in delivered, 'Duplicate slot accounting')
            skipped.add(slot)
    require(delivered | skipped == set(range(COUNT)), 'Unreported missing input slots')
    require(abs(inputs[0]['y']) <= .51/1.75, 'Input did not begin at the first viewport')
    require(not packet['nativeMotion']['overflow'], 'Native motion trace overflow')
    return dict(acceptedFixedEarlyInputScope=True, readinessTriggered=False,
                navigationToPlannedInputMs=DELAY_NS/1e6,
                navigationToActualFirstInputMs=(inputs[0]['timeNs']-navigation['navigationSentNs'])/1e6,
                armMissedStartDeadline=spec['armedNs'] > start,
                plannedInputs=COUNT, deliveredInputs=len(delivered), skippedInputs=len(skipped),
                allPlannedInputDelivered=not skipped, completeDeliveredSlotSequence=not skipped,
                latenessMs=distribution(lateness),
                entriesAtInput=dict(Counter(row['entries'] for row in inputs)),
                inputsBeforeTargetPath=sum(path(row['path']) != path(navigation['target'])
                                          for row in inputs),
                unacceptedInputs=sum(not row['accepted'] for row in inputs),
                actualInputStopLatenessMs=(spec['inputFinishedNs']-start-DURATION_NS)/1e6,
                cacheBefore=packet['cacheBefore'], cacheAfter=packet['cacheAfter'],
                osFileCacheCold=False, allEntryWarmTourVerified=False,
                physicalMotionVerified=False, fullPerformanceAccepted=False)
