import copy
import unittest

from fixed_early_input import verify, COUNT, DELAY_NS, PERIOD_NS, DURATION_NS, DIRECTION_SLOTS


class FixedEarlyInputTests(unittest.TestCase):
    def setUp(self):
        self.nav = dict(initializedBeforeNavigation=True, bothPanels=True,
                        action='panel.navigatePath', pid=4, renderedFramesBeforeNavigation=3,
                        navigationSentNs=3_000_000_000, firstObservedSwapNs=0,
                        sourcePaths={'0': 'empty', '1': 'empty'},
                        cacheBeforeNavigation=dict(frames=0, stores=0, pending=0),
                        target='C:/Users/xs/Downloads', active=True, nativeForeground=True,
                        dpr=1.75, hz=160, **{'from': 'empty'})
        self.startup = dict(emptyAtStartup=True, configuredBeforeQml=True, cacheRoot='cache', pid=4)
        start = self.nav['navigationSentNs']+DELAY_NS
        wheel = [dict(slot=i, scheduledNs=start+i*PERIOD_NS, timeNs=start+i*PERIOD_NS,
                      latenessNs=0, up=(i//DIRECTION_SLOTS) % 2 == 1, active=True,
                      nativeForeground=True, dpr=1.75, hz=160, density=152,
                      mode='masonry', entries=192, path=self.nav['target'], accepted=True, y=0)
                 for i in range(COUNT)]
        self.packet = dict(coldCacheRoot='cache', allEntryWarmTourVerified=False, warmTour={},
                           middle=False, coldPositioned=True, startNs=start,
                           endNs=start+DURATION_NS+100, wheel=wheel,
                           cacheBefore={}, cacheAfter={}, nativeMotion=dict(overflow=False),
                           fixedSchedule=dict(startDelayNs=DELAY_NS, periodNs=PERIOD_NS,
                               durationNs=DURATION_NS, count=COUNT, directionSlots=DIRECTION_SLOTS,
                               readinessTriggered=False, catchUpBursts=False, skippedSlots=[],
                               armedNs=start-100, firstDispatchNs=start,
                               inputFinishedNs=start+DURATION_NS))

    def test_complete_input_is_admitted_without_claiming_performance(self):
        result = verify(self.packet, self.startup, self.nav)
        self.assertTrue(result['allPlannedInputDelivered'])
        self.assertFalse(result['fullPerformanceAccepted'])

    def test_late_missing_slot_is_retained_and_disqualifies_identical_delivery(self):
        row = self.packet['wheel'].pop(1)
        self.packet['fixedSchedule']['skippedSlots'] = [dict(firstSlot=1, count=1,
                                                           observedNs=row['scheduledNs'])]
        result = verify(self.packet, self.startup, self.nav)
        self.assertEqual(result['skippedInputs'], 1)
        self.assertFalse(result['completeDeliveredSlotSequence'])
        self.packet['fixedSchedule']['skippedSlots'] = []
        with self.assertRaises(ValueError):
            verify(self.packet, self.startup, self.nav)

    def test_early_loading_inputs_are_counted_instead_of_filtered(self):
        self.packet['wheel'][0].update(entries=1, path='empty', accepted=False)
        result = verify(self.packet, self.startup, self.nav)
        self.assertEqual(result['entriesAtInput'][1], 1)
        self.assertEqual(result['inputsBeforeTargetPath'], 1)
        self.assertEqual(result['unacceptedInputs'], 1)

    def test_readiness_shift_direction_and_false_missing_slots_are_rejected(self):
        for mutate in [lambda p: p.update(startNs=p['startNs']+1),
                       lambda p: p['wheel'][150].update(up=False),
                       lambda p: p['fixedSchedule'].update(readinessTriggered=True),
                       lambda p: p['fixedSchedule'].update(skippedSlots=[
                           dict(firstSlot=1, count=1, observedNs=p['startNs']+PERIOD_NS)])]:
            packet = copy.deepcopy(self.packet)
            mutate(packet)
            with self.assertRaises(ValueError):
                verify(packet, self.startup, self.nav)

    def test_route_focus_and_truncation_are_rejected(self):
        for mutate in [lambda p: p['wheel'][0].update(nativeForeground=False),
                       lambda p: p['wheel'][0].update(path='unrelated'),
                       lambda p: p['wheel'][-1].update(entries=1),
                       lambda p: p['fixedSchedule'].update(inputFinishedNs=p['startNs'])]:
            packet = copy.deepcopy(self.packet)
            mutate(packet)
            with self.assertRaises(ValueError):
                verify(packet, self.startup, self.nav)


if __name__ == '__main__':
    unittest.main()
