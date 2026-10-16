import unittest
from collect import summarize

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

if __name__ == '__main__': unittest.main()
