import importlib.util
import unittest
from decimal import Decimal
from pathlib import Path

spec = importlib.util.spec_from_file_location('retail', Path(__file__).with_name('openrouter-retail-plan.py'))
retail = importlib.util.module_from_spec(spec)
spec.loader.exec_module(retail)

class RetailPlanTest(unittest.TestCase):
    def test_component_caps_preserve_lower_output_and_cache(self):
        old = {'models':[{'status':'enabled','model':'vendor/test','official_price':{'prompt':1,'completion':4,'input_cache_read':.1}}]}
        gemini = {'exchange_rate':{'usd_cny':6.652882205513785},'models':[]}
        snapshot = {'options':{'Price':'0.2','ModelRatio':'{"vendor/test":17}','CompletionRatio':'{"vendor/test":2}','CacheRatio':'{"vendor/test":0.01}'}}
        plan = retail.build_plan(old,gemini,snapshot)
        row=plan['models'][0]['energy_per_million']
        self.assertLessEqual(row['prompt']*.2,6.652882205513785)
        self.assertAlmostEqual(row['completion'],68,places=8)
        self.assertAlmostEqual(row['input_cache_read'],.34,places=8)
        self.assertEqual(snapshot['options']['ModelRatio'],'{"vendor/test":17}')

    def test_audio_and_image_units(self):
        gemini={'exchange_rate':{'usd_cny':6.6},'models':[
            {'channel_id':1638,'model':'tts','price_verified':True,'official_price':{'prompt':.5,'completion':6},'output_modalities':['speech']},
            {'channel_id':1638,'model':'image','price_verified':True,'official_price':{'prompt':2,'completion':12,'image_output':120,'input_cache_read':.2},'output_modalities':['text','image']},
            {'channel_id':1638,'model':'fixed','price_verified':True,'official_price':{'prompt':.25,'completion':1.5},'output_modalities':['image']},
        ]}
        result=retail.build_plan({'models':[]},gemini,{'options':{'Price':'0.2','ModelPrice':'{"fixed":1}'}})
        changes=result['option_changes']
        self.assertEqual(changes['AudioRatio']['tts']['after'],1)
        self.assertEqual(changes['AudioCompletionRatio']['tts']['after'],12)
        self.assertIn('img_o * 3960',result['billing_expressions']['image'])
        self.assertIn('cr * 6.6',result['billing_expressions']['image'])
        self.assertNotIn('fixed',changes['ModelRatio'])
        self.assertEqual(result['models'][-1]['action'],'preserve_existing_fixed_price')

    def test_null_cache_not_invented(self):
        result=retail.build_plan({'models':[{'status':'enabled','model':'vendor/m','official_price':{'prompt':1,'completion':2,'input_cache_read':None}}]}, {'exchange_rate':{'usd_cny':6.6},'models':[]},{'options':{'Price':'0.2'}})
        self.assertNotIn('CacheRatio',result['option_changes'])

if __name__=='__main__':unittest.main()
