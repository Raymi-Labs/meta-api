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

import { bg } from 'date-fns/locale';

const locale = {
  code: 'bg-BG',
  dateFnsLocale: bg,
  Pagination: {
    pageSize: 'Елементи на страница: ${pageSize}',
    total: 'Общо: ${total}',
    jumpTo: 'Към',
    page: ' страница',
  },
  Modal: { confirm: 'Потвърждаване', cancel: 'Отказ' },
  Tabs: { more: 'Още' },
  TimePicker: {
    placeholder: {
      time: 'Изберете час',
      timeRange: 'Изберете времеви интервал',
    },
    begin: 'Начален час',
    end: 'Краен час',
    hour: 'ч',
    minute: 'мин',
    second: 'сек',
    AM: 'преди обяд',
    PM: 'следобед',
  },
  DatePicker: {
    placeholder: {
      date: 'Изберете дата',
      dateTime: 'Изберете дата и час',
      dateRange: ['Начална дата', 'Крайна дата'],
      dateTimeRange: ['Начална дата', 'Крайна дата'],
      monthRange: ['Начален месец', 'Краен месец'],
    },
    presets: 'Готови варианти',
    footer: { confirm: 'Потвърждаване', cancel: 'Отказ' },
    selectDate: 'Избор на дата',
    selectTime: 'Избор на час',
    year: 'година',
    month: 'месец',
    day: 'ден',
    monthText: '${month} ${year}',
    months: {
      1: 'яну',
      2: 'фев',
      3: 'март',
      4: 'апр',
      5: 'май',
      6: 'юни',
      7: 'юли',
      8: 'авг',
      9: 'сеп',
      10: 'окт',
      11: 'ное',
      12: 'дек',
    },
    fullMonths: {
      1: 'Януари',
      2: 'Февруари',
      3: 'Март',
      4: 'Април',
      5: 'Май',
      6: 'Юни',
      7: 'Юли',
      8: 'Август',
      9: 'Септември',
      10: 'Октомври',
      11: 'Ноември',
      12: 'Декември',
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
  Popconfirm: { confirm: 'Потвърждаване', cancel: 'Отказ' },
  Navigation: {
    collapseText: 'Свиване на страничната лента',
    expandText: 'Разгъване на страничната лента',
  },
  Table: {
    emptyText: 'Няма резултати',
    pageText: 'Показване на ${currentStart} – ${currentEnd} от ${total}',
    descend: 'Натиснете за низходящо сортиране',
    ascend: 'Натиснете за възходящо сортиране',
    cancelSort: 'Отмяна на сортирането',
  },
  Select: { emptyText: 'Няма резултати', createText: 'Създаване' },
  Tree: { emptyText: 'Няма резултати', searchPlaceholder: 'Търсене' },
  Cascader: { emptyText: 'Няма резултати' },
  List: { emptyText: 'Няма резултати' },
  Calendar: {
    allDay: 'Цял ден',
    AM: '${time} преди обяд',
    PM: '${time} следобед',
    datestring: '',
    remaining: 'Още ${remained}',
  },
  Upload: {
    mainText: 'Натиснете, за да качите файл, или го плъзнете тук',
    illegalTips: 'Този тип файл не се поддържа',
    legalTips: 'Пуснете файла, за да започне качването',
    retry: 'Повторен опит',
    replace: 'Замяна на файла',
    clear: 'Изчистване',
    selectedFiles: 'Избрани файлове',
    illegalSize: 'Недопустим размер на файла',
    fail: 'Качването е неуспешно',
  },
  TreeSelect: { searchPlaceholder: 'Търсене' },
  Typography: {
    copy: 'Копиране',
    copied: 'Копирано',
    expand: 'Разгъване',
    collapse: 'Свиване',
  },
  Transfer: {
    emptyLeft: 'Няма данни',
    emptySearch: 'Няма резултати от търсенето',
    emptyRight: 'Няма избрани елементи. Изберете отляво',
    placeholder: 'Търсене',
    clear: 'Изчистване',
    selectAll: 'Избиране на всички',
    clearSelectAll: 'Премахване на всички отметки',
    total: 'Общо елементи: ${total}',
    selected: 'Избрани елементи: ${total}',
  },
  Form: { optional: '(незадължително)' },
  Image: {
    preview: 'Преглед',
    loading: 'Зареждане',
    loadError: 'Зареждането е неуспешно',
    prevTip: 'Предишно',
    nextTip: 'Следващо',
    zoomInTip: 'Увеличаване',
    zoomOutTip: 'Намаляване',
    rotateTip: 'Завъртане',
    downloadTip: 'Изтегляне',
    adaptiveTip: 'Побиране в страницата',
    originTip: 'Оригинален размер',
  },
  Chat: {
    deleteConfirm: 'Сигурни ли сте, че искате да изтриете тази сесия?',
    clearContext: 'Контекстът е изчистен',
    copySuccess: 'Успешно копиране.',
    stop: 'Спиране',
    copy: 'Копиране',
    copied: 'Копирано',
    dropAreaText: 'Пуснете файла тук',
  },
};

export default locale;
