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

import React, { useMemo } from 'react';
import { Card, Table, Empty } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { getPricingTableColumns } from './PricingTableColumns';

const PricingTable = ({
  filteredModels,
  loading,
  rowSelection,
  pageSize,
  setPageSize,
  selectedGroup,
  groupRatio,
  timeRatioMap,
  copyText,
  setModalImageUrl,
  setIsModalOpenurl,
  currency,
  siteDisplayType,
  currentPage,
  setCurrentPage,
  tokenUnit,
  displayPrice,
  showRatio,
  compactMode = false,
  openModelDetail,
  catalogState,
  t,
}) => {
  const columns = useMemo(() => {
    return getPricingTableColumns({
      t,
      selectedGroup,
      groupRatio,
      timeRatioMap,
      copyText,
      setModalImageUrl,
      setIsModalOpenurl,
      currency,
      tokenUnit,
      displayPrice,
      showRatio,
      siteDisplayType,
      openModelDetail,
      catalogState,
    });
  }, [
    t,
    selectedGroup,
    groupRatio,
    timeRatioMap,
    copyText,
    setModalImageUrl,
    setIsModalOpenurl,
    currency,
    tokenUnit,
    displayPrice,
    showRatio,
    siteDisplayType,
    openModelDetail,
    catalogState,
  ]);

  // Search is already applied by the shared hook, including descriptions/tags.
  const processedColumns = useMemo(() => {
    // Remove fixed property when in compact mode (mobile view)
    if (compactMode) {
      return columns.map(({ fixed, ...rest }) => rest);
    }
    return columns;
  }, [columns, compactMode]);

  // Semi treats controlled pagination as server pagination and leaves the
  // supplied rows untouched. Slice the filtered catalog before passing it in.
  const pageModels = useMemo(
    () =>
      filteredModels.slice(
        (currentPage - 1) * pageSize,
        currentPage * pageSize,
      ),
    [filteredModels, currentPage, pageSize],
  );

  const ModelTable = useMemo(
    () => (
      <Card className='!rounded-xl overflow-hidden' bordered={false}>
        <Table
          className='pricing-model-table'
          columns={processedColumns}
          dataSource={pageModels}
          loading={loading}
          rowSelection={rowSelection}
          scroll={compactMode ? undefined : { x: 'max-content' }}
          onRow={(record) => ({
            onClick: () => openModelDetail && openModelDetail(record),
            style: { cursor: 'pointer' },
          })}
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('搜索无结果')}
              style={{ padding: 30 }}
            />
          }
          pagination={{
            defaultPageSize: 20,
            pageSize: pageSize,
            currentPage,
            total: filteredModels.length,
            onPageChange: setCurrentPage,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 50, 100],
            onPageSizeChange: (size) => {
              setPageSize(size);
              setCurrentPage(1);
            },
          }}
        />
      </Card>
    ),
    [
      filteredModels,
      pageModels,
      loading,
      processedColumns,
      rowSelection,
      pageSize,
      currentPage,
      setCurrentPage,
      setPageSize,
      openModelDetail,
      t,
      compactMode,
    ],
  );

  return ModelTable;
};

export default PricingTable;
