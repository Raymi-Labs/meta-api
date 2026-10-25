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

import { uk } from 'date-fns/locale';

const locale = {
  code: 'uk-UA',
  dateFnsLocale: uk,
  Pagination: {
    pageSize: 'Елементів на сторінці: ${pageSize}',
    total: 'Усього: ${total}',
    jumpTo: 'Перейти до',
    page: ' сторінки',
  },
  Modal: { confirm: 'Підтвердити', cancel: 'Скасувати' },
  Tabs: { more: 'Ще' },
  TimePicker: {
    placeholder: { time: 'Виберіть час', timeRange: 'Виберіть проміжок часу' },
    begin: 'Час початку',
    end: 'Час завершення',
    hour: 'год',
    minute: 'хв',
    second: 'с',
    AM: 'до полудня',
    PM: 'після полудня',
  },
  DatePicker: {
    placeholder: {
      date: 'Виберіть дату',
      dateTime: 'Виберіть дату й час',
      dateRange: ['Дата початку', 'Дата завершення'],
      dateTimeRange: ['Дата початку', 'Дата завершення'],
      monthRange: ['Початковий місяць', 'Кінцевий місяць'],
    },
    presets: 'Готові варіанти',
    footer: { confirm: 'Підтвердити', cancel: 'Скасувати' },
    selectDate: 'Вибрати дату',
    selectTime: 'Вибрати час',
    year: 'рік',
    month: 'місяць',
    day: 'день',
    monthText: '${month} ${year}',
    months: {
      1: 'січ.',
      2: 'лют.',
      3: 'бер.',
      4: 'квіт.',
      5: 'трав.',
      6: 'черв.',
      7: 'лип.',
      8: 'серп.',
      9: 'вер.',
      10: 'жовт.',
      11: 'лист.',
      12: 'груд.',
    },
    fullMonths: {
      1: 'Січень',
      2: 'Лютий',
      3: 'Березень',
      4: 'Квітень',
      5: 'Травень',
      6: 'Червень',
      7: 'Липень',
      8: 'Серпень',
      9: 'Вересень',
      10: 'Жовтень',
      11: 'Листопад',
      12: 'Грудень',
    },
    weeks: {
      Mon: 'Пн',
      Tue: 'Вт',
      Wed: 'Ср',
      Thu: 'Чт',
      Fri: 'Пт',
      Sat: 'Сб',
      Sun: 'Нд',
    },
    localeFormatToken: { FORMAT_SWITCH_DATE: 'dd.MM.yyyy' },
  },
  Popconfirm: { confirm: 'Підтвердити', cancel: 'Скасувати' },
  Navigation: {
    collapseText: 'Згорнути бічну панель',
    expandText: 'Розгорнути бічну панель',
  },
  Table: {
    emptyText: 'Немає результатів',
    pageText: 'Показано від ${currentStart} до ${currentEnd} із ${total}',
    descend: 'Натисніть для сортування за спаданням',
    ascend: 'Натисніть для сортування за зростанням',
    cancelSort: 'Скасувати сортування',
  },
  Select: { emptyText: 'Немає результатів', createText: 'Створити' },
  Tree: { emptyText: 'Немає результатів', searchPlaceholder: 'Пошук' },
  Cascader: { emptyText: 'Немає результатів' },
  List: { emptyText: 'Немає результатів' },
  Calendar: {
    allDay: 'Увесь день',
    AM: '${time} до полудня',
    PM: '${time} після полудня',
    datestring: '',
    remaining: 'Ще ${remained}',
  },
  Upload: {
    mainText: 'Натисніть, щоб завантажити файл, або перетягніть його сюди',
    illegalTips: 'Цей тип файлу не підтримується',
    legalTips: 'Відпустіть, щоб почати завантаження',
    retry: 'Спробувати ще раз',
    replace: 'Замінити файл',
    clear: 'Очистити',
    selectedFiles: 'Вибрані файли',
    illegalSize: 'Неприпустимий розмір файлу',
    fail: 'Не вдалося завантажити файл',
  },
  TreeSelect: { searchPlaceholder: 'Пошук' },
  Typography: {
    copy: 'Копіювати',
    copied: 'Скопійовано',
    expand: 'Розгорнути',
    collapse: 'Згорнути',
  },
  Transfer: {
    emptyLeft: 'Немає даних',
    emptySearch: 'Нічого не знайдено',
    emptyRight: 'Нічого не вибрано. Виберіть елементи ліворуч',
    placeholder: 'Пошук',
    clear: 'Очистити',
    selectAll: 'Вибрати все',
    clearSelectAll: 'Скасувати вибір усіх',
    total: 'Усього елементів: ${total}',
    selected: 'Вибрано елементів: ${total}',
  },
  Form: { optional: '(необов’язково)' },
  Image: {
    preview: 'Попередній перегляд',
    loading: 'Завантаження',
    loadError: 'Не вдалося завантажити',
    prevTip: 'Попереднє',
    nextTip: 'Наступне',
    zoomInTip: 'Збільшити',
    zoomOutTip: 'Зменшити',
    rotateTip: 'Повернути',
    downloadTip: 'Завантажити',
    adaptiveTip: 'За розміром сторінки',
    originTip: 'Початковий розмір',
  },
  Chat: {
    deleteConfirm: 'Ви впевнені, що хочете видалити цей сеанс?',
    clearContext: 'Контекст очищено',
    copySuccess: 'Успішно скопійовано.',
    stop: 'Зупинити',
    copy: 'Копіювати',
    copied: 'Скопійовано',
    dropAreaText: 'Перетягніть файл сюди',
  },
};

export default locale;
