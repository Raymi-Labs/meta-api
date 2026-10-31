/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React from 'react';
import { Layout, ImagePreview } from '@douyinfe/semi-ui';
import PricingCatalogFilters from './header/PricingCatalogFilters';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import './pricingCatalog.css';
import PricingContent from './content/PricingContent';
import PricingTopSection from './header/PricingTopSection';
import { useModelPricingData } from '../../../../hooks/model-pricing/useModelPricingData';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  buildModelDetailPath,
  sanitizeModelCatalogSnapshot,
} from '../../../../helpers/modelDetailNavigation';

const PricingPage = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [initialState] = React.useState(() =>
    sanitizeModelCatalogSnapshot(location.state?.modelCatalogSnapshot),
  );
  const pricingData = useModelPricingData(initialState);
  const { Content } = Layout;
  const isMobile = useIsMobile();
  const [showRatio, setShowRatio] = React.useState(initialState.showRatio);
  const [viewMode, setViewMode] = React.useState(initialState.viewMode);
  const catalogState = sanitizeModelCatalogSnapshot({
    ...pricingData,
    showRatio,
    viewMode,
  });
  const snapshotKey = JSON.stringify(catalogState);
  React.useEffect(() => {
    if (JSON.stringify(location.state?.modelCatalogSnapshot) === snapshotKey)
      return;
    navigate(location.pathname + location.search + location.hash, {
      replace: true,
      state: {
        ...location.state,
        modelCatalogSnapshot: JSON.parse(snapshotKey),
      },
    });
  }, [
    snapshotKey,
    location.pathname,
    location.search,
    location.hash,
    location.state,
    navigate,
  ]);
  const resultsRef = React.useRef(null);
  const changePage = (page) => {
    pricingData.setCurrentPage(page);
    resultsRef.current?.scrollIntoView({ block: 'start' });
  };
  const allProps = {
    ...pricingData,
    catalogState,
    openModelDetail: (model) =>
      navigate(buildModelDetailPath(model.model_name), {
        state: { catalogState },
      }),
    setCurrentPage: changePage,
    showRatio,
    setShowRatio,
    viewMode,
    setViewMode,
    defaultViewMode: 'card',
  };

  return (
    <div className='pricing-page pricing-catalog'>
      <header className='pricing-catalog-heading'>
        <div>
          <div className='pricing-catalog-title'>
            <h1>{pricingData.t('模型广场')}</h1>
            <span className='pricing-title-count'>
              {pricingData.t('共 {{count}} 个模型', {
                count: pricingData.models.length,
              })}
            </span>
          </div>
        </div>
        <Link to='/docs/api-examples'>
          {pricingData.t('API 文档')} <span aria-hidden='true'>↗</span>
        </Link>
      </header>
      <PricingTopSection {...allProps} sidebarProps={allProps} />
      <PricingCatalogFilters {...allProps} />
      <Layout className='pricing-layout'>
        <Content className='pricing-scroll-hide pricing-content'>
          <PricingContent
            {...allProps}
            resultsRef={resultsRef}
            isMobile={isMobile}
            sidebarProps={allProps}
          />
        </Content>
      </Layout>

      <ImagePreview
        src={pricingData.modalImageUrl}
        visible={pricingData.isModalOpenurl}
        onVisibleChange={(visible) => pricingData.setIsModalOpenurl(visible)}
      />
    </div>
  );
};

export default PricingPage;
