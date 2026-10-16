import unittest
from collect import summarize, cpu_usage, memory_usage, rolling_usage, month_remaining

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
        self.assertEqual(result['recent'], {'rx': 500100, 'tx': 300})
        self.assertEqual(result['observed_seconds'], 86400)
        self.assertEqual(result['total'], {'rx': 99, 'tx': 100})
    def test_no_data_or_no_elapsed_time(self):
        self.assertIsNone(summarize(None))
        self.assertIsNone(summarize(self.source(300, 300, []))['forecast_30d'])
    def test_valid_zero_traffic(self):
        result = summarize(self.source(0, 300, [self.row(0, 0, 0)]))
        self.assertEqual(result['forecast_30d'], {'rx': 0, 'tx': 0})

class RollingWeekTest(unittest.TestCase):
    def test_168_hours_excludes_old_days(self):
        rows = [{'timestamp': h*3600, 'rx': 100, 'tx': 200} for h in range(240)]
        measured, seconds = rolling_usage(rows, 0, 240*3600, 168*3600, 3600)
        self.assertEqual(seconds,168*3600)
        self.assertEqual(measured, {'rx':16800,'tx':33600})
    def test_partial_boundaries_and_short_sample(self):
        rows=[{'timestamp':0,'rx':100,'tx':200}]
        self.assertEqual(rolling_usage(rows,1800,2700,604800,3600),({'rx':100,'tx':200},900))
        self.assertEqual(rolling_usage(rows,0,3600,1800,3600),({'rx':50,'tx':100},1800))
    def test_month_end_and_december(self):
        from datetime import datetime,timezone,timedelta
        tz=timezone(timedelta(hours=8))
        stamp=int(datetime(2026,12,31,12,tzinfo=tz).timestamp())
        self.assertEqual(month_remaining(stamp),12*3600)
        month={'date':{'year':2026,'month':12},'rx':1000,'tx':2000}
        data={'created':{'timestamp':stamp-1800},'updated':{'timestamp':stamp,'date':{'year':2026,'month':12,'day':31}},
              'traffic':{'hour':[{'timestamp':stamp-3600,'rx':100,'tx':200}], 'month':[month]}}
        result=summarize(data)
        self.assertEqual(result['forecast_month'], {'rx':3400,'tx':6800})
        self.assertEqual(result['forecast_observed_seconds'],1800)

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
