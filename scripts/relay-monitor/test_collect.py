import unittest
from collect import summarize, cpu_usage, memory_usage

class ForecastTest(unittest.TestCase):
    def source(self, created, updated, rows):
        return {'created': {'timestamp': created}, 'updated': {'timestamp': updated},
                'traffic': {'fiveminute': rows, 'total': {'rx': 99, 'tx': 100}}}
    def row(self, timestamp, rx=100, tx=200):
        return {'timestamp': timestamp, 'rx': rx, 'tx': tx}
    def test_initial_partial_bucket(self):
        result = summarize(self.source(150, 300, [self.row(0)]))
        self.assertEqual(result['observed_seconds'], 150)
        self.assertEqual(result['forecast_30d']['rx'], 100 * 30 * 86400 // 150)
    def test_excludes_old_partial_bucket(self):
        result = summarize(self.source(0, 86550, [self.row(0, 999999), self.row(300)]))
        self.assertEqual(result['recent'], {'rx': 100, 'tx': 200})
        self.assertEqual(result['observed_seconds'], 86250)
        self.assertEqual(result['total'], {'rx': 99, 'tx': 100})
    def test_no_data_or_no_elapsed_time(self):
        self.assertIsNone(summarize(None))
        self.assertIsNone(summarize(self.source(300, 300, []))['forecast_30d'])
    def test_valid_zero_traffic(self):
        result = summarize(self.source(0, 300, [self.row(0, 0, 0)]))
        self.assertEqual(result['forecast_30d'], {'rx': 0, 'tx': 0})

class ServerStatusTest(unittest.TestCase):
    def test_cpu_excludes_idle_and_iowait(self):
        self.assertEqual(cpu_usage([0]*8, [20,0,10,60,10,0,0,0]), 30)
    def test_cpu_invalid_window(self):
        self.assertIsNone(cpu_usage([0]*8, [0]*8))
        self.assertIsNone(cpu_usage([1]*8, [0]*8))
    def test_available_memory_includes_reclaimable_cache(self):
        result = memory_usage('MemTotal: 1000 kB\nMemFree: 100 kB\nMemAvailable: 700 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB')
        self.assertEqual(result['used'], 300*1024)
        self.assertEqual(result['percent'], 30)
        self.assertEqual(result['swap_used'], 0)

if __name__ == '__main__': unittest.main()
